package metricsprovider_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pipewave-dev/go-pkg/export/types"
	metricsprovider "github.com/pipewave-dev/go-pkg/provider/metrics-provider"
	"github.com/stretchr/testify/require"
)

func TestProvider_DisabledIsInert(t *testing.T) {
	p, err := metricsprovider.NewStandalone(&types.MetricsT{Enabled: false}, "test-container")
	require.NoError(t, err)
	require.Nil(t, p.Handler())
	require.NoError(t, p.ListenAndServe()) // must not block, must not error
	require.NotNil(t, p.Metrics())         // still returns usable no-op metrics
	require.NoError(t, p.Shutdown(context.Background()))
	require.NoError(t, p.Shutdown(context.Background())) // repeated calls must also return nil
}

// TestProvider_ShutdownIsIdempotent pins the bug where a second call to
// Shutdown surfaced sdkmetric.MeterProvider's "reader is shutdown" error even
// though the shutdown itself is a no-op after the first call. Shutdown is
// registered as a DI cleanup task AND may be called explicitly by the
// container's main() (Task 9), so it must tolerate being called more than
// once.
func TestProvider_ShutdownIsIdempotent(t *testing.T) {
	p, err := metricsprovider.NewStandalone(&types.MetricsT{
		Enabled: true, Port: 0, Path: "/metrics",
	}, "test-container")
	require.NoError(t, err)

	firstErr := p.Shutdown(context.Background())
	require.NoError(t, firstErr, "first Shutdown call must succeed")

	secondErr := p.Shutdown(context.Background())
	require.NoError(t, secondErr, "second Shutdown call must return nil, not surface the SDK's post-shutdown error")
}

func TestProvider_EnabledServesMetrics(t *testing.T) {
	p, err := metricsprovider.NewStandalone(&types.MetricsT{
		Enabled: true, Port: 0, Path: "/metrics",
	}, "test-container")
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	// Record something so at least one pipewave series exists.
	p.Metrics().RecordConnectionAccepted(context.Background(), "ws", "user")

	srv := httptest.NewServer(p.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "pipewave_connections_accepted_total")
}
