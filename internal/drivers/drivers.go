package drivers

import (
	"context"
	"log/slog"
	"time"

	"github.com/tseiman/HomeAuthMonitorGW/internal/cache"
	"github.com/tseiman/HomeAuthMonitorGW/internal/metrics"
)

type Collector interface {
	Poll(context.Context) ([]metrics.Metric, error)
}
type Discoverer interface {
	Discover(context.Context) ([]metrics.Definition, error)
}

func Run(ctx context.Context, name string, interval time.Duration, collector Collector, store *cache.Store, logger *slog.Logger) {
	poll := func() {
		start := time.Now()
		values, err := collector.Poll(ctx)
		duration := time.Since(start)
		before, _ := store.Snapshot(name)
		if err != nil {
			store.Failure(name, err, duration)
			if logger != nil && before.Available {
				logger.Warn("collector unavailable", "source", name)
			}
		} else {
			store.Success(name, values, duration)
			if logger != nil && !before.Available {
				logger.Info("collector recovered", "source", name)
			}
		}
	}
	poll()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}
