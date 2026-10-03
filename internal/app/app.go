package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tseiman/HomeAuthMonitorGW/internal/cache"
	"github.com/tseiman/HomeAuthMonitorGW/internal/config"
	"github.com/tseiman/HomeAuthMonitorGW/internal/drivers"
	"github.com/tseiman/HomeAuthMonitorGW/internal/drivers/nut"
	snmpdriver "github.com/tseiman/HomeAuthMonitorGW/internal/drivers/snmp"
	"github.com/tseiman/HomeAuthMonitorGW/internal/tlsreload"
)

type sourceSpec struct {
	name, driver    string
	interval, stale time.Duration
	collector       drivers.Collector
	discoverer      drivers.Discoverer
}
type candidate struct {
	cfg   config.Config
	cert  *tls.Certificate
	specs []sourceSpec
}
type Runtime struct {
	Store  *cache.Store
	TLS    *tlsreload.Manager
	path   string
	logger *slog.Logger
	active atomic.Pointer[config.Config]
	token  atomic.Value
	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func Check(path string) error {
	_, err := prepare(path)
	return err
}

func New(path string, logger *slog.Logger) (*Runtime, error) {
	cand, err := prepare(path)
	if err != nil {
		return nil, err
	}
	tlsManager, err := tlsreload.New(cand.cfg.Server.Certificate, cand.cfg.Server.PrivateKey)
	if err != nil {
		return nil, err
	}
	r := &Runtime{Store: cache.New(time.Now), TLS: tlsManager, path: path, logger: logger}
	r.active.Store(&cand.cfg)
	r.token.Store(cand.cfg.Secrets.BearerToken)
	r.start(cand.specs)
	return r, nil
}
func prepare(path string) (candidate, error) {
	cfg, err := config.Load(path, true)
	if err != nil {
		return candidate{}, err
	}
	cert, err := tlsreload.Load(cfg.Server.Certificate, cfg.Server.PrivateKey)
	if err != nil {
		return candidate{}, err
	}
	specs, err := prepareSources(cfg)
	if err != nil {
		return candidate{}, err
	}
	return candidate{cfg: cfg, cert: cert, specs: specs}, nil
}
func prepareSources(cfg config.Config) ([]sourceSpec, error) {
	var out []sourceSpec
	if cfg.Collectors.NUT.Enabled {
		c := nut.Collector{Client: nut.Client{Address: cfg.Collectors.NUT.Server, Timeout: cfg.Collectors.NUT.Timeout.Duration}, UPS: cfg.Collectors.NUT.UPS}
		out = append(out, sourceSpec{name: "quint", driver: "nut", interval: cfg.Collectors.NUT.PollInterval.Duration, stale: cfg.Collectors.NUT.StaleAfter.Duration, collector: c})
	}
	if cfg.Collectors.WAGO.Enabled {
		meta, err := snmpdriver.LoadMetadata(cfg.Collectors.WAGO.MetadataFile)
		if err != nil {
			return nil, err
		}
		g, err := snmpdriver.NewGoSNMP(cfg.Collectors.WAGO.Address, cfg.Collectors.WAGO.Port, cfg.Collectors.WAGO.SNMP.Username, cfg.Secrets.SNMPAuthPassphrase, cfg.Secrets.SNMPPrivPassphrase, cfg.Collectors.WAGO.Timeout.Duration)
		if err != nil {
			return nil, err
		}
		reader := &snmpdriver.GoSNMPReader{Template: g}
		c := &snmpdriver.Collector{Reader: reader, OIDs: cfg.Collectors.WAGO.OIDs, Roots: cfg.Collectors.WAGO.Discovery.RootOIDs, Metadata: meta}
		var d drivers.Discoverer
		if cfg.Collectors.WAGO.Discovery.Enabled {
			d = c
		}
		out = append(out, sourceSpec{name: "wago", driver: "snmp", interval: cfg.Collectors.WAGO.PollInterval.Duration, stale: cfg.Collectors.WAGO.StaleAfter.Duration, collector: c, discoverer: d})
	}
	return out, nil
}
func (r *Runtime) Reload() error {
	cand, err := prepare(r.path)
	if err != nil {
		return err
	}
	old := r.active.Load()
	if old != nil && !reloadCompatible(*old, cand.cfg) {
		return errors.New("server listener, timeouts, header limit, and health visibility require restart")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopLocked()
	r.TLS.Activate(cand.cert)
	r.active.Store(&cand.cfg)
	r.token.Store(cand.cfg.Secrets.BearerToken)
	r.startLocked(cand.specs)
	return nil
}
func reloadCompatible(a, b config.Config) bool {
	return a.Server.Listen == b.Server.Listen && a.Server.ReadTimeout == b.Server.ReadTimeout && a.Server.WriteTimeout == b.Server.WriteTimeout && a.Server.IdleTimeout == b.Server.IdleTimeout && a.Server.ShutdownTimeout == b.Server.ShutdownTimeout && a.Server.MaxHeaderBytes == b.Server.MaxHeaderBytes && a.Authentication.HealthPublic == b.Authentication.HealthPublic
}
func (r *Runtime) start(specs []sourceSpec) { r.mu.Lock(); defer r.mu.Unlock(); r.startLocked(specs) }
func (r *Runtime) startLocked(specs []sourceSpec) {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.Store.Reset()
	for _, s := range specs {
		r.Store.Register(s.name, s.driver, s.stale)
		r.wg.Add(1)
		go func(s sourceSpec) {
			defer r.wg.Done()
			drivers.Run(ctx, s.name, s.interval, s.collector, r.Store, r.logger)
		}(s)
		if s.discoverer != nil {
			r.wg.Add(1)
			go func(s sourceSpec) {
				defer r.wg.Done()
				dctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				defer cancel()
				defs, err := s.discoverer.Discover(dctx)
				if err != nil {
					if r.logger != nil {
						r.logger.Warn("source discovery failed", "source", s.name)
					}
					return
				}
				r.Store.SetDiscovery(s.name, defs)
			}(s)
		}
	}
}
func (r *Runtime) stopLocked() {
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	r.wg.Wait()
}
func (r *Runtime) Close() { r.mu.Lock(); defer r.mu.Unlock(); r.stopLocked() }
func (r *Runtime) Token() string {
	v := r.token.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}
func (r *Runtime) Config() config.Config {
	p := r.active.Load()
	if p == nil {
		return config.Config{}
	}
	return *p
}
func (r *Runtime) ValidateActive() error {
	if r.active.Load() == nil {
		return fmt.Errorf("no active configuration")
	}
	return nil
}
