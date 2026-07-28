package moduledelivery

import "context"

// ServeMetrics starts the metrics listener; no-op when metrics are disabled.
func (m *moduleDelivery) ServeMetrics() error {
	return m.metricsProvider.ListenAndServe()
}

// ShutdownMetrics stops the metrics listener.
func (m *moduleDelivery) ShutdownMetrics(ctx context.Context) error {
	return m.metricsProvider.Shutdown(ctx)
}

// CallbackObserver returns the webhook call observer, or nil when metrics are
// disabled. Typed as any so core/delivery does not import server/webhook.
func (m *moduleDelivery) CallbackObserver() any {
	if m.callbackMetrics == nil {
		return nil
	}
	return m.callbackMetrics
}
