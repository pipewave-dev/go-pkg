package metrics

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// meterName scopes every pipewave instrument.
const meterName = "github.com/pipewave-dev/go-pkg"

// Label values. Each dimension is a closed set so cardinality stays bounded.
const (
	TransportWS       = "ws"
	TransportLongPoll = "longpoll"

	AuthAnon = "anon"
	AuthUser = "user"

	RejectMissingToken   = "missing_token"
	RejectInvalidToken   = "invalid_token"
	RejectUpgradeFailed  = "upgrade_failed"
	RejectRegisterFailed = "register_failed"

	OutcomeOK            = "ok"
	OutcomeError         = "error"
	OutcomeInvalidSchema = "invalid_schema"
	OutcomeDedup         = "dedup"
	OutcomeRateLimited   = "rate_limited"
)

// Config carries the process-level values needed to build instruments.
type Config struct {
	// MsgTypeAllowlist bounds the msg_type label; see SanitizeMsgType.
	MsgTypeAllowlist []string
	Version          string
	ContainerID      string
}

// PipewaveMetrics holds the Tier 1 instruments: connection lifecycle and
// inbound client messages.
type PipewaveMetrics struct {
	connAccepted metric.Int64Counter
	connRejected metric.Int64Counter
	connDuration metric.Float64Histogram
	clientMsgs   metric.Int64Counter
	clientMsgDur metric.Float64Histogram

	msgTypeAllowlist map[string]struct{}
}

// New builds the Tier 1 instruments from the global MeterProvider.
//
// It deliberately does NOT create or install a MeterProvider: the embedding
// process owns that choice. With no provider configured the OTEL API hands back
// no-op instruments, so this is safe and free to call unconditionally.
//
// Instrument creation errors are logged and downgraded to no-op instruments —
// metrics must never take down the main path.
func New(cfg Config) *PipewaveMetrics {
	meter := otel.GetMeterProvider().Meter(meterName)

	m := &PipewaveMetrics{
		msgTypeAllowlist: BuildAllowlist(cfg.MsgTypeAllowlist),
	}

	m.connAccepted = mustCounter(meter, "pipewave_connections_accepted_total",
		"Total WebSocket/long-poll connections accepted")
	m.connRejected = mustCounter(meter, "pipewave_connections_rejected_total",
		"Total connection attempts rejected, by reason")
	m.clientMsgs = mustCounter(meter, "pipewave_client_messages_total",
		"Total inbound client messages, by type and outcome")

	m.connDuration = mustHistogram(meter, "pipewave_connection_duration_seconds",
		"Lifetime of a client connection in seconds")
	m.clientMsgDur = mustHistogram(meter, "pipewave_client_message_duration_seconds",
		"Time spent handling one inbound client message")

	m.registerBuildInfo(meter, cfg)

	return m
}

// registerBuildInfo publishes a constant 1 carrying version/container_id.
// container_id is deliberately confined to this metric: as a label on a counter
// or histogram it would multiply every series by the number of pods.
func (m *PipewaveMetrics) registerBuildInfo(meter metric.Meter, cfg Config) {
	g, err := meter.Int64ObservableGauge("pipewave_build_info",
		metric.WithDescription("Always 1; labels carry build and container identity"))
	if err != nil {
		slog.Warn("metrics: create pipewave_build_info failed", slog.Any("error", err))
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("version", cfg.Version),
		attribute.String("container_id", cfg.ContainerID),
	)
	if _, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(g, 1, attrs)
		return nil
	}, g); err != nil {
		slog.Warn("metrics: register pipewave_build_info callback failed", slog.Any("error", err))
	}
}

// mustCounter never fails: on error it logs and returns a no-op instrument.
func mustCounter(meter metric.Meter, name, desc string) metric.Int64Counter {
	c, err := meter.Int64Counter(name, metric.WithDescription(desc))
	if err != nil {
		slog.Warn("metrics: create counter failed", slog.String("name", name), slog.Any("error", err))
		c, _ = noop.NewMeterProvider().Meter(meterName).Int64Counter(name)
	}
	return c
}

// mustHistogram never fails: on error it logs and returns a no-op instrument.
func mustHistogram(meter metric.Meter, name, desc string) metric.Float64Histogram {
	h, err := meter.Float64Histogram(name,
		metric.WithDescription(desc),
		metric.WithUnit("s"))
	if err != nil {
		slog.Warn("metrics: create histogram failed", slog.String("name", name), slog.Any("error", err))
		h, _ = noop.NewMeterProvider().Meter(meterName).Float64Histogram(name)
	}
	return h
}

// RecordConnectionAccepted counts one admitted connection.
func (m *PipewaveMetrics) RecordConnectionAccepted(ctx context.Context, transport, auth string) {
	m.connAccepted.Add(ctx, 1, metric.WithAttributes(
		attribute.String("transport", transport),
		attribute.String("auth", auth),
	))
}

// RecordConnectionRejected counts one refused connection attempt. reason must
// be one of the Reject* constants.
func (m *PipewaveMetrics) RecordConnectionRejected(ctx context.Context, transport, reason string) {
	m.connRejected.Add(ctx, 1, metric.WithAttributes(
		attribute.String("transport", transport),
		attribute.String("reason", reason),
	))
}

// RecordConnectionDuration records how long a connection stayed open.
func (m *PipewaveMetrics) RecordConnectionDuration(ctx context.Context, seconds float64, auth string) {
	m.connDuration.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("auth", auth),
	))
}

// RecordClientMessage counts one inbound message and records its handling time.
// rawMsgType is the client-supplied wire value and is sanitized here, so
// callers may pass it through unmodified.
func (m *PipewaveMetrics) RecordClientMessage(ctx context.Context, rawMsgType, outcome string, seconds float64) {
	msgType := SanitizeMsgType(rawMsgType, m.msgTypeAllowlist)
	m.clientMsgs.Add(ctx, 1, metric.WithAttributes(
		attribute.String("msg_type", msgType),
		attribute.String("outcome", outcome),
	))
	m.clientMsgDur.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("msg_type", msgType),
	))
}
