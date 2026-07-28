package webhook_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

func TestHealthMonitor_FiresOnceOnUnhealthy(t *testing.T) {
	var fired atomic.Int64
	m := webhook.NewHealthMonitor(func() { fired.Add(1) })
	require.True(t, m.IsHealthy())
	m.SetUnhealthy("boom")
	m.SetUnhealthy("boom again")
	require.False(t, m.IsHealthy())
	require.Equal(t, int64(1), fired.Load())
}

func TestHealthMonitor_SetHealthyClearsFlagButNotRefire(t *testing.T) {
	var fired atomic.Int64
	m := webhook.NewHealthMonitor(func() { fired.Add(1) })
	m.SetHealthy() // no-op when already healthy
	require.True(t, m.IsHealthy())
	require.Equal(t, int64(0), fired.Load())
}

func TestWatchBreakerOpen_FiresWhenOpenTooLong(t *testing.T) {
	b := webhook.NewCircuitBreaker(1, time.Hour) // long cooldown → stays open
	b.Record(false)                              // open now
	var fired atomic.Int64
	m := webhook.NewHealthMonitor(func() { fired.Add(1) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go webhook.WatchBreakerOpen(ctx, b, 5*time.Millisecond, m)
	require.Eventually(t, func() bool { return fired.Load() >= 1 }, time.Second, time.Millisecond)
}
