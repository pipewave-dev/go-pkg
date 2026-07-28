package metrics_test

import (
	"context"
	"testing"

	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newTestReader installs a fresh MeterProvider backed by a ManualReader and
// restores the previous global provider when the test ends.
func newTestReader(t *testing.T) *metric.ManualReader {
	t.Helper()
	reader := metric.NewManualReader()
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(metric.NewMeterProvider(metric.WithReader(reader)))
	t.Cleanup(func() { otel.SetMeterProvider(prev) })
	return reader
}

func collect(t *testing.T, r *metric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, r.Collect(context.Background(), &rm))
	return rm
}

// findMetric returns the named metric from any scope, failing the test if absent.
func findMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Metrics{}
}

func sumFor(t *testing.T, m metricdata.Metrics, want map[string]string) int64 {
	t.Helper()
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "metric %s is not an int64 sum", m.Name)
	for _, dp := range sum.DataPoints {
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

func TestRecordConnectionAccepted(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{Version: "v1", ContainerID: "c1"})

	m.RecordConnectionAccepted(context.Background(), metrics.TransportWS, metrics.AuthUser)
	m.RecordConnectionAccepted(context.Background(), metrics.TransportWS, metrics.AuthUser)
	m.RecordConnectionAccepted(context.Background(), metrics.TransportLongPoll, metrics.AuthAnon)

	got := findMetric(t, collect(t, reader), "pipewave_connections_accepted_total")
	require.Equal(t, int64(2), sumFor(t, got, map[string]string{"transport": "ws", "auth": "user"}))
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{"transport": "longpoll", "auth": "anon"}))
}

func TestRecordConnectionRejected(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	m.RecordConnectionRejected(context.Background(), metrics.TransportWS, metrics.RejectInvalidToken)

	got := findMetric(t, collect(t, reader), "pipewave_connections_rejected_total")
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{"transport": "ws", "reason": "invalid_token"}))
}

func TestRecordClientMessage_SanitizesMsgType(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{MsgTypeAllowlist: []string{"CHAT"}})

	// allowlisted -> kept
	m.RecordClientMessage(context.Background(), "CHAT", metrics.OutcomeOK, 0.01)
	// not allowlisted -> "other"
	m.RecordClientMessage(context.Background(), "SECRET", metrics.OutcomeOK, 0.02)
	// system heartbeat byte -> "heartbeat"
	m.RecordClientMessage(context.Background(), string([]byte{202}), metrics.OutcomeOK, 0.03)

	got := findMetric(t, collect(t, reader), "pipewave_client_messages_total")
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{"msg_type": "CHAT"}))
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{"msg_type": "other"}))
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{"msg_type": "heartbeat"}))
}

func TestRecordClientMessage_RecordsHistogram(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	m.RecordClientMessage(context.Background(), string([]byte{202}), metrics.OutcomeOK, 0.25)

	got := findMetric(t, collect(t, reader), "pipewave_client_message_duration_seconds")
	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, hist.DataPoints, 1)
	require.Equal(t, uint64(1), hist.DataPoints[0].Count)
	require.InDelta(t, 0.25, hist.DataPoints[0].Sum, 0.0001)
}

func TestRecordConnectionDuration(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	m.RecordConnectionDuration(context.Background(), 12.5, metrics.AuthUser)

	got := findMetric(t, collect(t, reader), "pipewave_connection_duration_seconds")
	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.InDelta(t, 12.5, hist.DataPoints[0].Sum, 0.0001)
}

func TestBuildInfo(t *testing.T) {
	reader := newTestReader(t)
	_ = metrics.New(metrics.Config{Version: "v0.0.1", ContainerID: "abc123"})

	got := findMetric(t, collect(t, reader), "pipewave_build_info")
	gauge, ok := got.Data.(metricdata.Gauge[int64])
	require.True(t, ok)
	require.Len(t, gauge.DataPoints, 1)
	require.Equal(t, int64(1), gauge.DataPoints[0].Value)
	v, found := gauge.DataPoints[0].Attributes.Value("version")
	require.True(t, found)
	require.Equal(t, "v0.0.1", v.AsString())
}

// A no-op global provider must not panic and must not require any config.
func TestNew_NoProviderIsSafe(t *testing.T) {
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(noop.NewMeterProvider())
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	m := metrics.New(metrics.Config{})
	require.NotPanics(t, func() {
		m.RecordConnectionAccepted(context.Background(), metrics.TransportWS, metrics.AuthUser)
		m.RecordConnectionRejected(context.Background(), metrics.TransportWS, metrics.RejectMissingToken)
		m.RecordConnectionDuration(context.Background(), 1, metrics.AuthUser)
		m.RecordClientMessage(context.Background(), "CHAT", metrics.OutcomeOK, 1)
	})
}
