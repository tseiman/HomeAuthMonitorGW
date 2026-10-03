package metrics

import "time"

type Metric struct {
	Name        string            `json:"name"`
	Value       any               `json:"value"`
	ValueType   string            `json:"value_type"`
	Unit        string            `json:"unit,omitempty"`
	Description string            `json:"description,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type Definition struct {
	OID         string            `json:"oid"`
	Name        string            `json:"name,omitempty"`
	Type        string            `json:"type,omitempty"`
	Unit        string            `json:"unit,omitempty"`
	Description string            `json:"description,omitempty"`
	Enum        map[string]string `json:"enum,omitempty"`
}
