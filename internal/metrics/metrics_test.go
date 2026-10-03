package metrics

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMetricJSONHasStableProtocolIndependentFields(t *testing.T) {
	m := Metric{Name: "input_voltage", Value: 230.5, ValueType: "number", Unit: "V", Description: "Input voltage", Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Labels: map[string]string{"phase": "L1"}}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"input_voltage","value":230.5,"value_type":"number","unit":"V","description":"Input voltage","timestamp":"2026-01-01T00:00:00Z","labels":{"phase":"L1"}}`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
}
