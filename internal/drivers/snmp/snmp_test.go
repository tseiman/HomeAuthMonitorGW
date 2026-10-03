package snmp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	gosnmp "github.com/gosnmp/gosnmp"
)

type fakeReader struct {
	getCalls, walkCalls int
	pdus                []gosnmp.SnmpPDU
}

func (f *fakeReader) Get(context.Context, []string) ([]gosnmp.SnmpPDU, error) {
	f.getCalls++
	return f.pdus, nil
}
func (f *fakeReader) Walk(_ context.Context, _ []string) ([]gosnmp.SnmpPDU, error) {
	f.walkCalls++
	return f.pdus, nil
}

func TestConvertPDUUsesMetadataAndSafeTypes(t *testing.T) {
	meta := Metadata{"1.3.6.1.2.1.1.3.0": {OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTime", Type: "TimeTicks", Unit: "centiseconds", Description: "Uptime"}}
	m, err := ConvertPDU(gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(123)}, meta, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "sysUpTime" || m.Value != uint64(123) || m.ValueType != "counter" || m.Unit != "centiseconds" {
		t.Fatalf("metric=%+v", m)
	}
}

func TestConvertPDURejectsUnsupportedValue(t *testing.T) {
	if _, err := ConvertPDU(gosnmp.SnmpPDU{Name: ".1", Type: gosnmp.NoSuchObject}, nil, time.Now()); err == nil {
		t.Fatal("expected error")
	}
}

func TestPollingAndDiscoveryAreSeparateReadOperations(t *testing.T) {
	f := &fakeReader{pdus: []gosnmp.SnmpPDU{{Name: ".1.2.3", Type: gosnmp.Integer, Value: 5}}}
	c := Collector{Reader: f, OIDs: []string{"1.2.3"}, Roots: []string{"1.2"}}
	if _, err := c.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.getCalls != 1 || f.walkCalls != 0 {
		t.Fatalf("get=%d walk=%d", f.getCalls, f.walkCalls)
	}
	defs, err := c.Discover(context.Background())
	if err != nil || len(defs) != 1 {
		t.Fatalf("defs=%v err=%v", defs, err)
	}
	if f.walkCalls != 1 {
		t.Fatalf("walk=%d", f.walkCalls)
	}
}

func TestLoadMetadataRejectsUnknownFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mib.json")
	b, _ := json.Marshal([]map[string]any{{"oid": "1.2.3", "name": "x", "unexpected": true}})
	os.WriteFile(p, b, 0o600)
	if _, err := LoadMetadata(p); err == nil {
		t.Fatal("expected unknown field error")
	}
}
