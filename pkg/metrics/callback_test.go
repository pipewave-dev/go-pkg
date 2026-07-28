package metrics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestCallbackMetrics_Duration(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	c.ObserveCall("inspect_token", "sync", 150*time.Millisecond, 200, nil)

	got := findMetric(t, collect(t, reader), "pipewave_callback_duration_seconds")
	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Equal(t, uint64(1), hist.DataPoints[0].Count)
	require.InDelta(t, 0.15, hist.DataPoints[0].Sum, 0.001)
}

func TestCallbackMetrics_NoErrorMetricOnSuccess(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	c.ObserveCall("inspect_token", "sync", time.Millisecond, 200, nil)

	rm := collect(t, reader)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "pipewave_callback_errors_total" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			require.Empty(t, sum.DataPoints, "a 200 must not record an error")
		}
	}
}

func TestCallbackMetrics_ErrorReasons(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	c.ObserveCall("handle_message", "sync", time.Millisecond, 500, nil)
	c.ObserveCall("handle_message", "sync", time.Millisecond, 0, context.DeadlineExceeded)
	c.ObserveCall("handle_message", "async", time.Millisecond, 404, nil)

	got := findMetric(t, collect(t, reader), "pipewave_callback_errors_total")
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{
		"event_type": "handle_message", "mode": "sync", "reason": "status_5xx"}))
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{
		"event_type": "handle_message", "mode": "sync", "reason": "timeout"}))
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{
		"event_type": "handle_message", "mode": "async", "reason": "status_4xx"}))
}

func TestCallbackMetrics_RetryAndDropped(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	c.ObserveRetry("on_close_connection", "async")
	c.ObserveRetry("on_close_connection", "async")
	c.ObserveDropped("on_close_connection")

	rm := collect(t, reader)
	retries := findMetric(t, rm, "pipewave_callback_retries_total")
	require.Equal(t, int64(2), sumFor(t, retries, map[string]string{
		"event_type": "on_close_connection", "mode": "async"}))

	dropped := findMetric(t, rm, "pipewave_callback_dropped_total")
	require.Equal(t, int64(1), sumFor(t, dropped, map[string]string{
		"event_type": "on_close_connection"}))
}

type stubBreaker struct {
	open  bool
	since time.Time
}

func (s *stubBreaker) OpenSince() (time.Time, bool) { return s.since, s.open }

func TestCallbackMetrics_BreakerGauge(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	br := &stubBreaker{open: false}
	require.NoError(t, c.RegisterBreakerGauge(br))

	got := findMetric(t, collect(t, reader), "pipewave_callback_breaker_open")
	g, ok := got.Data.(metricdata.Gauge[int64])
	require.True(t, ok)
	require.Equal(t, int64(0), g.DataPoints[0].Value)

	br.open = true
	br.since = time.Now()
	got = findMetric(t, collect(t, reader), "pipewave_callback_breaker_open")
	g, _ = got.Data.(metricdata.Gauge[int64])
	require.Equal(t, int64(1), g.DataPoints[0].Value)
}

func TestCallbackMetrics_UnknownErrorIsOther(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	c.ObserveCall("ping", "sync", time.Millisecond, 0, errors.New("boom"))

	got := findMetric(t, collect(t, reader), "pipewave_callback_errors_total")
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{
		"event_type": "ping", "mode": "sync", "reason": "other"}))
}
