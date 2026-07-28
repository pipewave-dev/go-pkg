package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pipewave-dev/go-pkg/core/service/business"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// statsCallbackTimeout bounds how long a scrape waits on the stats source.
// The callback runs on the Prometheus scrape path; without a bound a slow
// source would let scrapes pile up.
const statsCallbackTimeout = 2 * time.Second

// StatsCallbackTimeoutForTest exposes statsCallbackTimeout to the metrics_test package.
func StatsCallbackTimeoutForTest() time.Duration { return statsCallbackTimeout }

// ConnectionStatsSource supplies live per-container connection counts.
// business.Monitoring satisfies it.
type ConnectionStatsSource interface {
	InsideActiveConnection(ctx context.Context) (*business.SumaryActiveConnection, aerror.AError)
}

// connStatsCache holds the last successful reading so a transient failure
// reports stale-but-plausible numbers instead of a gap in the series.
type connStatsCache struct {
	mu    sync.Mutex
	value *business.SumaryActiveConnection
}

func (c *connStatsCache) get() *business.SumaryActiveConnection {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

func (c *connStatsCache) set(v *business.SumaryActiveConnection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = v
}

// RegisterConnectionGauges publishes pipewave_connections_active and
// pipewave_users_active, read from src on every scrape.
//
// These are ObservableGauges, not UpDownCounters, on purpose: a counter that is
// incremented on connect and decremented on disconnect drifts permanently the
// first time a decrement is missed (panic, crash, cancelled context). Reading
// live state each scrape is self-correcting.
//
// The values are per-container (InsideActiveConnection). Each pod reports its
// own share and Prometheus sum() aggregates; using a cluster-wide total here
// would over-count by the number of pods.
func (m *PipewaveMetrics) RegisterConnectionGauges(src ConnectionStatsSource) error {
	meter := otel.GetMeterProvider().Meter(meterName)

	active, err := meter.Int64ObservableGauge("pipewave_connections_active",
		metric.WithDescription("Active connections held by this container, by auth kind"))
	if err != nil {
		return err
	}
	users, err := meter.Int64ObservableGauge("pipewave_users_active",
		metric.WithDescription("Distinct users connected to this container"))
	if err != nil {
		return err
	}

	cache := &connStatsCache{}
	anonAttrs := metric.WithAttributes(attribute.String("auth", AuthAnon))
	userAttrs := metric.WithAttributes(attribute.String("auth", AuthUser))

	_, err = meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		ctx, cancel := context.WithTimeout(ctx, statsCallbackTimeout)
		defer cancel()

		sum, aErr := src.InsideActiveConnection(ctx)
		if aErr != nil || sum == nil {
			// Fall back to the previous reading; a failed scrape must not
			// surface as a zero, which would look like an outage.
			sum = cache.get()
			if sum == nil {
				slog.Warn("metrics: connection stats unavailable and no cached value",
					slog.Any("error", aErr))
				return nil
			}
		} else {
			cache.set(sum)
		}

		o.ObserveInt64(active, int64(sum.AnonymosConnection), anonAttrs)
		o.ObserveInt64(active, int64(sum.UserConnection), userAttrs)
		o.ObserveInt64(users, int64(sum.TotalUser))
		return nil
	}, active, users)

	return err
}
