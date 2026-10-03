package drivers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tseiman/HomeAuthMonitorGW/internal/cache"
	"github.com/tseiman/HomeAuthMonitorGW/internal/metrics"
)

type fakeCollector struct{ calls int }

func (f *fakeCollector) Poll(context.Context) ([]metrics.Metric, error) {
	f.calls++
	if f.calls == 1 {
		return []metrics.Metric{{Name: "ok", Value: true}}, nil
	}
	return nil, errors.New("down")
}

func TestRunnerPollsImmediatelyAndContainsFailure(t *testing.T) {
	store := cache.New(time.Now)
	store.Register("source", "fake", time.Minute)
	f := &fakeCollector{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { Run(ctx, "source", 10*time.Millisecond, f, store, nil); close(done) }()
	time.Sleep(25 * time.Millisecond)
	cancel()
	<-done
	got, _ := store.Snapshot("source")
	if f.calls < 2 || got.Available || !got.Stale || len(got.Metrics) != 1 {
		t.Fatalf("calls=%d snapshot=%+v", f.calls, got)
	}
}
