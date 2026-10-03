package tlsreload

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

type Manager struct {
	current atomic.Pointer[tls.Certificate]
}

func New(certFile, keyFile string) (*Manager, error) {
	c, err := Load(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	m := &Manager{}
	m.Activate(c)
	return m, nil
}
func Load(certFile, keyFile string) (*tls.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS keypair: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return nil, errors.New("TLS certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse TLS leaf: %w", err)
	}
	pair.Leaf = leaf
	return &pair, nil
}
func (m *Manager) Activate(c *tls.Certificate) { m.current.Store(c) }
func (m *Manager) Reload(certFile, keyFile string) error {
	c, err := Load(certFile, keyFile)
	if err != nil {
		return err
	}
	m.Activate(c)
	return nil
}
func (m *Manager) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c := m.current.Load()
	if c == nil {
		return nil, errors.New("no TLS certificate loaded")
	}
	return c, nil
}
func (m *Manager) NotAfter() time.Time {
	c := m.current.Load()
	if c == nil || c.Leaf == nil {
		return time.Time{}
	}
	return c.Leaf.NotAfter
}
