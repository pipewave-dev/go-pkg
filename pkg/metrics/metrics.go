package metrics

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// PipewaveMetrics holds all OTEL metrics instruments for the Pipewave SDK.
type PipewaveMetrics struct {
	activeConnections metric.Int64UpDownCounter
	messagesSent      metric.Int64Counter
	messagesReceived  metric.Int64Counter
	connDuration      metric.Float64Histogram
	pubsubMessages    metric.Int64Counter
}

// New creates and registers all Pipewave metrics instruments.
func New() *PipewaveMetrics {
	exporter, err := prometheus.New()
	if err != nil {
		panic(err)
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	meter := provider.Meter("pipewave")

	m := &PipewaveMetrics{}

	m.activeConnections, _ = meter.Int64UpDownCounter("pipewave_active_connections",
		metric.WithDescription("Number of active WebSocket connections"))
	m.messagesSent, _ = meter.Int64Counter("pipewave_messages_sent_total",
		metric.WithDescription("Total messages sent"))
	m.messagesReceived, _ = meter.Int64Counter("pipewave_messages_received_total",
		metric.WithDescription("Total messages received from clients"))
	m.connDuration, _ = meter.Float64Histogram("pipewave_connection_duration_seconds",
		metric.WithDescription("Duration of WebSocket connections"))
	m.pubsubMessages, _ = meter.Int64Counter("pipewave_pubsub_messages_total",
		metric.WithDescription("Total pub/sub messages published"))

	return m
}

// RecordConnectionOpen increments the active connection counter.
func (m *PipewaveMetrics) RecordConnectionOpen(ctx context.Context, connType string) {
	m.activeConnections.Add(ctx, 1, metric.WithAttributes(
		attribute.String("type", connType),
	))
}

// RecordConnectionClose decrements the active connection counter.
func (m *PipewaveMetrics) RecordConnectionClose(ctx context.Context, connType string) {
	m.activeConnections.Add(ctx, -1, metric.WithAttributes(
		attribute.String("type", connType),
	))
}

// RecordMessageSent increments the messages sent counter.
func (m *PipewaveMetrics) RecordMessageSent(ctx context.Context, target string) {
	m.messagesSent.Add(ctx, 1, metric.WithAttributes(
		attribute.String("target", target),
	))
}

// RecordMessageReceived increments the messages received counter.
func (m *PipewaveMetrics) RecordMessageReceived(ctx context.Context) {
	m.messagesReceived.Add(ctx, 1)
}

// RecordConnectionDuration records the duration of a WebSocket connection.
func (m *PipewaveMetrics) RecordConnectionDuration(ctx context.Context, seconds float64, connType string) {
	m.connDuration.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("type", connType),
	))
}

// RecordPubsubMessage increments the pub/sub messages counter.
func (m *PipewaveMetrics) RecordPubsubMessage(ctx context.Context) {
	m.pubsubMessages.Add(ctx, 1)
}
