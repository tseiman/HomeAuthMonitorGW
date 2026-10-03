package snmp

import (
	"context"
	"fmt"
	"sync"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
)

func NewGoSNMP(target string, port uint16, username, authPassphrase, privacyPassphrase string, timeout time.Duration) (*gosnmp.GoSNMP, error) {
	if target == "" || username == "" || authPassphrase == "" || privacyPassphrase == "" {
		return nil, fmt.Errorf("SNMPv3 authPriv parameters are required")
	}
	return &gosnmp.GoSNMP{Target: target, Port: port, Version: gosnmp.Version3, Timeout: timeout, Retries: 1, ExponentialTimeout: true, MsgFlags: gosnmp.AuthPriv, SecurityModel: gosnmp.UserSecurityModel, SecurityParameters: &gosnmp.UsmSecurityParameters{UserName: username, AuthenticationProtocol: gosnmp.SHA, AuthenticationPassphrase: authPassphrase, PrivacyProtocol: gosnmp.DES, PrivacyPassphrase: privacyPassphrase}, MaxOids: 32, MaxRepetitions: 20}, nil
}

type GoSNMPReader struct {
	Template *gosnmp.GoSNMP
	mu       sync.Mutex
}

func (r *GoSNMPReader) connect(ctx context.Context) error {
	if r.Template == nil {
		return fmt.Errorf("nil SNMP configuration")
	}
	r.Template.Context = ctx
	if err := r.Template.Connect(); err != nil {
		return fmt.Errorf("connect SNMP: %w", err)
	}
	return nil
}
func (r *GoSNMPReader) Get(ctx context.Context, oids []string) ([]gosnmp.SnmpPDU, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.connect(ctx); err != nil {
		return nil, err
	}
	defer r.Template.Conn.Close()
	packet, err := r.Template.Get(oids)
	if err != nil {
		return nil, fmt.Errorf("SNMP get: %w", err)
	}
	if packet.Error != gosnmp.NoError {
		return nil, fmt.Errorf("SNMP response error %s", packet.Error.String())
	}
	return packet.Variables, nil
}
func (r *GoSNMPReader) Walk(ctx context.Context, roots []string) ([]gosnmp.SnmpPDU, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.connect(ctx); err != nil {
		return nil, err
	}
	defer r.Template.Conn.Close()
	var out []gosnmp.SnmpPDU
	for _, root := range roots {
		err := r.Template.BulkWalk(root, func(p gosnmp.SnmpPDU) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				out = append(out, p)
				return nil
			}
		})
		if err != nil {
			return nil, fmt.Errorf("SNMP walk: %w", err)
		}
	}
	return out, nil
}
