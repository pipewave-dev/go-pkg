package webhook_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

func TestPinger_Ping_2xxIsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	p := webhook.NewPinger(webhook.NewSender(srv.URL, newTestSigner(t)), time.Second, 3)
	require.NoError(t, p.Ping(context.Background()))
}

func TestPinger_Ping_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	p := webhook.NewPinger(webhook.NewSender(srv.URL, newTestSigner(t)), time.Second, 3)
	require.Error(t, p.Ping(context.Background()))
}

func TestPinger_Run_FiresUnhealthyAfterThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	var unhealthy atomic.Int64
	p := webhook.NewPinger(webhook.NewSender(srv.URL, newTestSigner(t)), time.Second, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx, time.Millisecond, func() {}, func() { unhealthy.Add(1); cancel() })

	require.Eventually(t, func() bool { return unhealthy.Load() >= 1 }, time.Second, time.Millisecond)
}

func TestPinger_Run_HealthyResetsStreak(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var healthy atomic.Int64
	p := webhook.NewPinger(webhook.NewSender(srv.URL, newTestSigner(t)), time.Second, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx, time.Millisecond, func() { healthy.Add(1) }, func() {})

	time.Sleep(10 * time.Millisecond)
	fail.Store(false)
	require.Eventually(t, func() bool { return healthy.Load() >= 1 }, time.Second, time.Millisecond)
}
