package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// CallbackMetrics instruments outbound webhook callbacks.
//
// It structurally satisfies webhook.CallObserver. The dependency points this
// way on purpose: server/webhook declares the interface and never imports this
// package, so webhook stays free of metrics plumbing.
type CallbackMetrics struct {
	duration metric.Float64Histogram
	errors   metric.Int64Counter
	retries  metric.Int64Counter
	dropped  metric.Int64Counter
}

// NewCallbackMetrics builds the Tier 2 callback instruments from the global
// MeterProvider. Safe when no provider is installed (no-op instruments).
func NewCallbackMetrics() *CallbackMetrics {
	meter := otel.GetMeterProvider().Meter(meterName)
	return &CallbackMetrics{
		duration: mustHistogram(meter, "pipewave_callback_duration_seconds",
			"Latency of one outbound callback attempt"),
		errors: mustCounter(meter, "pipewave_callback_errors_total",
			"Total failed callback attempts, by reason"),
		retries: mustCounter(meter, "pipewave_callback_retries_total",
			"Total callback retry attempts"),
		dropped: mustCounter(meter, "pipewave_callback_dropped_total",
			"Total callbacks abandoned after exhausting retries"),
	}
}

// ObserveCall records one completed callback attempt. A successful attempt
// records only the duration; errors additionally increment errors_total with a
// bounded reason label.
func (c *CallbackMetrics) ObserveCall(eventType, mode string, dur time.Duration, statusCode int, err error) {
	c.duration.Record(context.Background(), dur.Seconds(), metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.String("mode", mode),
	))

	reason := ClassifyCallbackError(err, statusCode)
	if reason == "" {
		return
	}
	c.errors.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.String("mode", mode),
		attribute.String("reason", reason),
	))
}

// ObserveRetry counts one retry attempt.
func (c *CallbackMetrics) ObserveRetry(eventType, mode string) {
	c.retries.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.String("mode", mode),
	))
}

// ObserveDropped counts one callback abandoned after retries.
func (c *CallbackMetrics) ObserveDropped(eventType string) {
	c.dropped.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("event_type", eventType),
	))
}

// BreakerStateSource reports circuit-breaker state.
// webhook.CircuitBreaker satisfies it via OpenSince.
type BreakerStateSource interface {
	OpenSince() (time.Time, bool)
}

// RegisterBreakerGauge publishes pipewave_callback_breaker_open as 0 or 1,
// read live on each scrape from the existing breaker state — no new state.
func (c *CallbackMetrics) RegisterBreakerGauge(src BreakerStateSource) error {
	meter := otel.GetMeterProvider().Meter(meterName)
	g, err := meter.Int64ObservableGauge("pipewave_callback_breaker_open",
		metric.WithDescription("1 when the callback circuit breaker is open, else 0"))
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		var v int64
		if _, open := src.OpenSince(); open {
			v = 1
		}
		o.ObserveInt64(g, v)
		return nil
	}, g)
	return err
}
