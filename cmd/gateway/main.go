package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/tseiman/HomeAuthMonitorGW/internal/app"
	"github.com/tseiman/HomeAuthMonitorGW/internal/httpapi"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("automation-gateway", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "/etc/automation-gateway/config.yaml", "configuration file")
	check := fs.Bool("check", false, "validate configuration and referenced files, then exit")
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "automation-gateway %s (commit %s, built %s)\n", version, commit, buildTime)
		return 0
	}
	if *check {
		if err := app.Check(*configPath); err != nil {
			fmt.Fprintf(stderr, "configuration invalid: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "configuration OK")
		return 0
	}
	if err := serve(*configPath, stderr); err != nil {
		fmt.Fprintf(stderr, "automation-gateway: %v\n", err)
		return 1
	}
	return 0
}
func serve(configPath string, logOutput io.Writer) error {
	bootstrap := slog.New(slog.NewJSONHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelInfo}))
	runtime, err := app.New(configPath, bootstrap)
	if err != nil {
		return err
	}
	defer runtime.Close()
	cfg := runtime.Config()
	level := slog.LevelInfo
	switch cfg.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(logOutput, &slog.HandlerOptions{Level: level}))
	handler := httpapi.New(runtime.Store, runtime.Token, httpapi.Options{Version: version, HealthPublic: cfg.Authentication.HealthPublic, CertificateNotAfter: runtime.TLS.NotAfter})
	server := &http.Server{Addr: cfg.Server.Listen, Handler: handler, ReadHeaderTimeout: cfg.Server.ReadTimeout.Duration, ReadTimeout: cfg.Server.ReadTimeout.Duration, WriteTimeout: cfg.Server.WriteTimeout.Duration, IdleTimeout: cfg.Server.IdleTimeout.Duration, MaxHeaderBytes: cfg.Server.MaxHeaderBytes, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: runtime.TLS.GetCertificate}}
	errs := make(chan error, 1)
	go func() {
		logger.Info("gateway starting", "version", version, "listen", cfg.Server.Listen)
		errs <- server.ListenAndServeTLS("", "")
	}()
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	for {
		select {
		case sig := <-signals:
			if sig == syscall.SIGHUP {
				if err := runtime.Reload(); err != nil {
					logger.Error("reload failed", "error", err)
				} else {
					logger.Info("configuration and certificate reloaded")
				}
				continue
			}
			logger.Info("shutdown requested", "signal", sig.String())
			ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout.Duration)
			err := server.Shutdown(ctx)
			cancel()
			if err != nil {
				return fmt.Errorf("graceful shutdown: %w", err)
			}
			err = <-errs
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case err := <-errs:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		}
	}
}
