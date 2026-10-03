package snmp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	gosnmp "github.com/gosnmp/gosnmp"
	"github.com/tseiman/HomeAuthMonitorGW/internal/metrics"
)

type Metadata map[string]metrics.Definition

func LoadMetadata(path string) (Metadata, error) {
	if path == "" {
		return Metadata{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var defs []metrics.Definition
	if err := dec.Decode(&defs); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}
	m := make(Metadata, len(defs))
	for _, d := range defs {
		oid := normalizeOID(d.OID)
		if oid == "" {
			return nil, errors.New("metadata OID is required")
		}
		if _, ok := m[oid]; ok {
			return nil, fmt.Errorf("duplicate metadata OID %s", oid)
		}
		d.OID = oid
		m[oid] = d
	}
	return m, nil
}

type Reader interface {
	Get(context.Context, []string) ([]gosnmp.SnmpPDU, error)
	Walk(context.Context, []string) ([]gosnmp.SnmpPDU, error)
}
type Collector struct {
	Reader      Reader
	OIDs, Roots []string
	Metadata    Metadata
	Now         func() time.Time
}

func (c Collector) Poll(ctx context.Context) ([]metrics.Metric, error) {
	pdus, err := c.Reader.Get(ctx, c.OIDs)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	out := make([]metrics.Metric, 0, len(pdus))
	for _, p := range pdus {
		m, e := ConvertPDU(p, c.Metadata, now)
		if e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, nil
}
func (c Collector) Discover(ctx context.Context) ([]metrics.Definition, error) {
	pdus, err := c.Reader.Walk(ctx, c.Roots)
	if err != nil {
		return nil, err
	}
	out := make([]metrics.Definition, 0, len(pdus))
	seen := map[string]bool{}
	for _, p := range pdus {
		oid := normalizeOID(p.Name)
		if seen[oid] {
			continue
		}
		seen[oid] = true
		if d, ok := c.Metadata[oid]; ok {
			out = append(out, d)
		} else {
			out = append(out, metrics.Definition{OID: oid, Name: oid, Type: p.Type.String()})
		}
	}
	return out, nil
}

func ConvertPDU(p gosnmp.SnmpPDU, metadata Metadata, at time.Time) (metrics.Metric, error) {
	oid := normalizeOID(p.Name)
	d, ok := metadata[oid]
	name := oid
	if ok && d.Name != "" {
		name = d.Name
	}
	var value any
	var typ string
	switch p.Type {
	case gosnmp.Integer:
		switch v := p.Value.(type) {
		case int:
			value = int64(v)
		case int64:
			value = v
		case uint:
			value = int64(v)
		default:
			value = gosnmp.ToBigInt(p.Value).Int64()
		}
		typ = "integer"
	case gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks:
		value = gosnmp.ToBigInt(p.Value).Uint64()
		typ = "counter"
	case gosnmp.Counter64:
		value = gosnmp.ToBigInt(p.Value).Uint64()
		typ = "counter"
	case gosnmp.OctetString:
		b, ok := p.Value.([]byte)
		if !ok {
			return metrics.Metric{}, errors.New("invalid octet string")
		}
		if utf8.Valid(b) {
			value = string(b)
			typ = "string"
		} else {
			value = fmt.Sprintf("%x", b)
			typ = "hex"
		}
	case gosnmp.ObjectIdentifier, gosnmp.IPAddress:
		value = fmt.Sprint(p.Value)
		typ = "string"
	default:
		return metrics.Metric{}, fmt.Errorf("unsupported SNMP value type %s", p.Type.String())
	}
	return metrics.Metric{Name: name, Value: value, ValueType: typ, Unit: d.Unit, Description: d.Description, Timestamp: at, Labels: map[string]string{"oid": oid}}, nil
}
func normalizeOID(s string) string { return strings.TrimPrefix(strings.TrimSpace(s), ".") }
