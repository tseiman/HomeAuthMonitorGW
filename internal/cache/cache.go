package cache

import (
	"sort"
	"sync"
	"time"

	"github.com/tseiman/HomeAuthMonitorGW/internal/metrics"
)

type Snapshot struct {
	Name         string               `json:"name"`
	Driver       string               `json:"driver"`
	Available    bool                 `json:"available"`
	Stale        bool                 `json:"stale"`
	LastAttempt  time.Time            `json:"last_attempt,omitempty"`
	LastSuccess  time.Time            `json:"last_success,omitempty"`
	PollDuration time.Duration        `json:"poll_duration_ns"`
	Error        string               `json:"error,omitempty"`
	Metrics      []metrics.Metric     `json:"metrics"`
	Discovery    []metrics.Definition `json:"discovery,omitempty"`
}

type entry struct {
	snapshot   Snapshot
	staleAfter time.Duration
}
type Store struct {
	mu  sync.RWMutex
	now func() time.Time
	m   map[string]*entry
}

func New(now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{now: now, m: make(map[string]*entry)}
}
func (s *Store) Register(name, driver string, staleAfter time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[name] = &entry{snapshot: Snapshot{Name: name, Driver: driver, Stale: true, Metrics: []metrics.Metric{}}, staleAfter: staleAfter}
}
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = make(map[string]*entry)
}
func (s *Store) Success(name string, values []metrics.Metric, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[name]
	if !ok {
		return
	}
	now := s.now().UTC()
	e.snapshot.Available, e.snapshot.Stale, e.snapshot.Error = true, false, ""
	e.snapshot.LastAttempt, e.snapshot.LastSuccess, e.snapshot.PollDuration = now, now, duration
	e.snapshot.Metrics = cloneMetrics(values)
}
func (s *Store) Failure(name string, _ error, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[name]
	if !ok {
		return
	}
	e.snapshot.Available, e.snapshot.Stale, e.snapshot.Error = false, true, "collector unavailable"
	e.snapshot.LastAttempt, e.snapshot.PollDuration = s.now().UTC(), duration
}
func (s *Store) SetDiscovery(name string, defs []metrics.Definition) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.m[name]; ok {
		e.snapshot.Discovery = append([]metrics.Definition(nil), defs...)
	}
}
func (s *Store) Snapshot(name string) (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.m[name]
	if !ok {
		return Snapshot{}, false
	}
	v := cloneSnapshot(e.snapshot)
	if !v.LastSuccess.IsZero() && s.now().Sub(v.LastSuccess) > e.staleAfter {
		v.Stale = true
	}
	return v, true
}
func (s *Store) All() []Snapshot {
	s.mu.RLock()
	names := make([]string, 0, len(s.m))
	for n := range s.m {
		names = append(names, n)
	}
	s.mu.RUnlock()
	sort.Strings(names)
	out := make([]Snapshot, 0, len(names))
	for _, n := range names {
		if v, ok := s.Snapshot(n); ok {
			out = append(out, v)
		}
	}
	return out
}
func cloneSnapshot(v Snapshot) Snapshot {
	v.Metrics = cloneMetrics(v.Metrics)
	v.Discovery = append([]metrics.Definition(nil), v.Discovery...)
	return v
}
func cloneMetrics(in []metrics.Metric) []metrics.Metric {
	out := append([]metrics.Metric(nil), in...)
	for i := range out {
		if out[i].Labels != nil {
			out[i].Labels = make(map[string]string, len(out[i].Labels))
			for k, v := range in[i].Labels {
				out[i].Labels[k] = v
			}
		}
	}
	return out
}
