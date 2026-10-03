package config

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	v, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	d.Duration = v
	return nil
}

type Config struct {
	Server         Server         `yaml:"server"`
	Authentication Authentication `yaml:"authentication"`
	Collectors     Collectors     `yaml:"collectors"`
	Logging        Logging        `yaml:"logging"`
	Secrets        Secrets        `yaml:"-"`
}
type Server struct {
	Listen          string   `yaml:"listen"`
	Certificate     string   `yaml:"certificate"`
	PrivateKey      string   `yaml:"private_key"`
	ReadTimeout     Duration `yaml:"read_timeout"`
	WriteTimeout    Duration `yaml:"write_timeout"`
	IdleTimeout     Duration `yaml:"idle_timeout"`
	ShutdownTimeout Duration `yaml:"shutdown_timeout"`
	MaxHeaderBytes  int      `yaml:"max_header_bytes"`
}
type Authentication struct {
	BearerTokenFile string `yaml:"bearer_token_file"`
	HealthPublic    bool   `yaml:"health_public"`
}
type Logging struct {
	Level string `yaml:"level"`
}
type Collectors struct {
	NUT  NUT  `yaml:"nut"`
	WAGO WAGO `yaml:"wago"`
}
type NUT struct {
	Enabled      bool     `yaml:"enabled"`
	Server       string   `yaml:"server"`
	UPS          string   `yaml:"ups"`
	PollInterval Duration `yaml:"poll_interval"`
	StaleAfter   Duration `yaml:"stale_after"`
	Timeout      Duration `yaml:"timeout"`
}
type WAGO struct {
	Enabled      bool      `yaml:"enabled"`
	Address      string    `yaml:"address"`
	Port         uint16    `yaml:"port"`
	PollInterval Duration  `yaml:"poll_interval"`
	StaleAfter   Duration  `yaml:"stale_after"`
	Timeout      Duration  `yaml:"timeout"`
	OIDs         []string  `yaml:"oids"`
	SNMP         SNMP      `yaml:"snmp"`
	Discovery    Discovery `yaml:"discovery"`
	MetadataFile string    `yaml:"metadata_file"`
}
type SNMP struct {
	Version               string `yaml:"version"`
	Username              string `yaml:"username"`
	AuthProtocol          string `yaml:"auth_protocol"`
	AuthPassphraseFile    string `yaml:"auth_passphrase_file"`
	PrivacyProtocol       string `yaml:"privacy_protocol"`
	PrivacyPassphraseFile string `yaml:"privacy_passphrase_file"`
}
type Discovery struct {
	Enabled  bool     `yaml:"enabled"`
	RootOIDs []string `yaml:"root_oids"`
}
type Secrets struct {
	BearerToken        string
	SNMPAuthPassphrase string
	SNMPPrivPassphrase string
}

func Load(path string, readSecrets bool) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return Config{}, errors.New("parse config: multiple YAML documents are not allowed")
	}
	c.defaults()
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	if readSecrets {
		if c.Secrets.BearerToken, err = readSecret(c.Authentication.BearerTokenFile); err != nil {
			return Config{}, fmt.Errorf("bearer token: %w", err)
		}
		if c.Collectors.WAGO.Enabled {
			if c.Secrets.SNMPAuthPassphrase, err = readSecret(c.Collectors.WAGO.SNMP.AuthPassphraseFile); err != nil {
				return Config{}, fmt.Errorf("SNMP auth passphrase: %w", err)
			}
			if c.Secrets.SNMPPrivPassphrase, err = readSecret(c.Collectors.WAGO.SNMP.PrivacyPassphraseFile); err != nil {
				return Config{}, fmt.Errorf("SNMP privacy passphrase: %w", err)
			}
		}
	}
	return c, nil
}

func (c *Config) defaults() {
	if c.Server.ReadTimeout.Duration == 0 {
		c.Server.ReadTimeout.Duration = 10 * time.Second
	}
	if c.Server.WriteTimeout.Duration == 0 {
		c.Server.WriteTimeout.Duration = 15 * time.Second
	}
	if c.Server.IdleTimeout.Duration == 0 {
		c.Server.IdleTimeout.Duration = 60 * time.Second
	}
	if c.Server.ShutdownTimeout.Duration == 0 {
		c.Server.ShutdownTimeout.Duration = 15 * time.Second
	}
	if c.Server.MaxHeaderBytes == 0 {
		c.Server.MaxHeaderBytes = 16 << 10
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Collectors.NUT.Timeout.Duration == 0 {
		c.Collectors.NUT.Timeout.Duration = 5 * time.Second
	}
	if c.Collectors.WAGO.Timeout.Duration == 0 {
		c.Collectors.WAGO.Timeout.Duration = 5 * time.Second
	}
	if c.Collectors.WAGO.Port == 0 {
		c.Collectors.WAGO.Port = 161
	}
}

func (c Config) Validate() error {
	if c.Server.Listen == "" {
		return errors.New("server.listen is required")
	}
	if _, _, err := net.SplitHostPort(c.Server.Listen); err != nil {
		return fmt.Errorf("server.listen: %w", err)
	}
	if c.Server.Certificate == "" {
		return errors.New("server.certificate is required")
	}
	if c.Server.PrivateKey == "" {
		return errors.New("server.private_key is required")
	}
	if c.Authentication.BearerTokenFile == "" {
		return errors.New("authentication.bearer_token_file is required")
	}
	if c.Server.MaxHeaderBytes != 0 && (c.Server.MaxHeaderBytes < 1024 || c.Server.MaxHeaderBytes > 1<<20) {
		return errors.New("server.max_header_bytes must be between 1024 and 1048576")
	}
	if c.Collectors.NUT.Enabled {
		if _, _, err := net.SplitHostPort(c.Collectors.NUT.Server); err != nil {
			return fmt.Errorf("collectors.nut.server: %w", err)
		}
		if c.Collectors.NUT.UPS == "" {
			return errors.New("collectors.nut.ups is required")
		}
		if err := validateTimes(c.Collectors.NUT.PollInterval.Duration, c.Collectors.NUT.StaleAfter.Duration, "collectors.nut"); err != nil {
			return err
		}
	}
	if c.Collectors.WAGO.Enabled {
		w := c.Collectors.WAGO
		if net.ParseIP(w.Address) == nil {
			return errors.New("collectors.wago.address must be an IP address")
		}
		if w.SNMP.Version != "3" {
			return errors.New("collectors.wago.snmp.version must be 3")
		}
		if w.SNMP.Username == "" || w.SNMP.AuthPassphraseFile == "" || w.SNMP.PrivacyPassphraseFile == "" {
			return errors.New("collectors.wago SNMP username and passphrase files are required")
		}
		if strings.ToUpper(w.SNMP.AuthProtocol) != "SHA" && strings.ToUpper(w.SNMP.AuthProtocol) != "SHA1" {
			return errors.New("collectors.wago.snmp.auth_protocol must be SHA")
		}
		if strings.ToUpper(w.SNMP.PrivacyProtocol) != "DES" {
			return errors.New("collectors.wago.snmp.privacy_protocol must be DES")
		}
		if err := validateTimes(w.PollInterval.Duration, w.StaleAfter.Duration, "collectors.wago"); err != nil {
			return err
		}
		if len(w.OIDs) == 0 {
			return errors.New("collectors.wago.oids must not be empty")
		}
		for _, oid := range append(append([]string{}, w.OIDs...), w.Discovery.RootOIDs...) {
			if !validOID(oid) {
				return fmt.Errorf("invalid configured OID %q", oid)
			}
		}
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("logging.level must be debug, info, warn, or error")
	}
	return nil
}

func validateTimes(poll, stale time.Duration, path string) error {
	if poll <= 0 {
		return fmt.Errorf("%s.poll_interval must be positive", path)
	}
	if stale < poll {
		return fmt.Errorf("%s.stale_after must be at least poll_interval", path)
	}
	return nil
}
func validOID(s string) bool {
	s = strings.TrimPrefix(s, ".")
	if s == "" {
		return false
	}
	for _, p := range strings.Split(s, ".") {
		if _, err := strconv.ParseUint(p, 10, 32); err != nil {
			return false
		}
	}
	return true
}
func readSecret(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	if info.Mode().Perm()&0o007 != 0 || info.Mode().Perm()&0o020 != 0 {
		return "", errors.New("insecure permissions: secret must not be accessible by others or group-writable")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return "", errors.New("file is empty")
	}
	return v, nil
}
