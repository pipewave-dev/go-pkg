package metrics_test

import (
	"context"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/core/service/business"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type stubStats struct {
	sum   *business.SumaryActiveConnection
	err   aerror.AError
	delay time.Duration
	calls int
}

func (s *stubStats) InsideActiveConnection(ctx context.Context) (*business.SumaryActiveConnection, aerror.AError) {
	s.calls++
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, aerror.New(ctx, aerror.ErrUnexpectedBussiness, ctx.Err())
		}
	}
	return s.sum, s.err
}

func gaugeFor(t *testing.T, m metricdata.Metrics, want map[string]string) int64 {
	t.Helper()
	g, ok := m.Data.(metricdata.Gauge[int64])
	require.True(t, ok, "metric %s is not an int64 gauge", m.Name)
	for _, dp := range g.DataPoints {
		matched := true
		for k, v := range want {
			got, found := dp.Attributes.Value(attribute.Key(k))
			if !found || got.AsString() != v {
				matched = false
				break
			}
		}
		if matched {
			return dp.Value
		}
	}
	t.Fatalf("no datapoint on %s matching %v", m.Name, want)
	return 0
}

func TestRegisterConnectionGauges(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	src := &stubStats{sum: &business.SumaryActiveConnection{
		AnonymosConnection: 3, UserConnection: 7, TotalUser: 5,
	}}
	require.NoError(t, m.RegisterConnectionGauges(src))

	rm := collect(t, reader)
	active := findMetric(t, rm, "pipewave_connections_active")
	require.Equal(t, int64(3), gaugeFor(t, active, map[string]string{"auth": "anon"}))
	require.Equal(t, int64(7), gaugeFor(t, active, map[string]string{"auth": "user"}))

	users := findMetric(t, rm, "pipewave_users_active")
	require.Equal(t, int64(5), gaugeFor(t, users, nil))
}

func TestRegisterConnectionGauges_ReflectsChanges(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	src := &stubStats{sum: &business.SumaryActiveConnection{AnonymosConnection: 1, UserConnection: 1, TotalUser: 1}}
	require.NoError(t, m.RegisterConnectionGauges(src))
	_ = collect(t, reader)

	// Gauges read live state each scrape, so a change must be visible without
	// any Record* call — this is what makes them drift-free.
	src.sum = &business.SumaryActiveConnection{AnonymosConnection: 42, UserConnection: 0, TotalUser: 9}
	active := findMetric(t, collect(t, reader), "pipewave_connections_active")
	require.Equal(t, int64(42), gaugeFor(t, active, map[string]string{"auth": "anon"}))
}

func TestRegisterConnectionGauges_ErrorKeepsLastGoodValue(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	src := &stubStats{sum: &business.SumaryActiveConnection{AnonymosConnection: 5, UserConnection: 5, TotalUser: 5}}
	require.NoError(t, m.RegisterConnectionGauges(src))
	_ = collect(t, reader) // primes the cache

	src.err = aerror.New(context.Background(), aerror.ErrUnexpectedBussiness, nil)
	src.sum = nil

	active := findMetric(t, collect(t, reader), "pipewave_connections_active")
	require.Equal(t, int64(5), gaugeFor(t, active, map[string]string{"auth": "anon"}))
}

func TestRegisterConnectionGauges_SlowSourceDoesNotBlockScrape(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	src := &stubStats{
		sum:   &business.SumaryActiveConnection{AnonymosConnection: 1, UserConnection: 1, TotalUser: 1},
		delay: 3 * time.Second,
	}
	require.NoError(t, m.RegisterConnectionGauges(src))

	start := time.Now()
	_ = collect(t, reader)
	elapsed := time.Since(start)

	// The source sleeps 3s; the callback's own timeout is 2s. Assert it bailed
	// out on the timeout rather than waiting for the source: comfortably under
	// the source delay, but not instant (which would mean the timeout never
	// engaged and something else returned early).
	require.Less(t, elapsed, 3*time.Second,
		"callback must bail out on its own timeout, not wait for the slow source")
	require.GreaterOrEqual(t, elapsed, metrics.StatsCallbackTimeoutForTest(),
		"callback returned before its timeout could have fired")
}
