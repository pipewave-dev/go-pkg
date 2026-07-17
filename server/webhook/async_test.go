package webhook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

// collectServer fails the first `failures` requests with 500, then accepts.
type collectServer struct {
	mu       sync.Mutex
	failures int
	got      []webhook.Body
}

func (c *collectServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env webhook.Body
		_ = json.Unmarshal(body, &env)
		c.mu.Lock()
		defer c.mu.Unlock()
		c.got = append(c.got, env)
		if c.failures > 0 {
			c.failures--
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (c *collectServer) envelopes() []webhook.Body {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]webhook.Body(nil), c.got...)
}

func tinyBackoff() []time.Duration { return []time.Duration{time.Millisecond} }

func TestAsyncDispatcher_RetriesUntilSuccess_SameCallbackID(t *testing.T) {
	cs := &collectServer{failures: 2}
	srv := httptest.NewServer(cs.handler())
	defer srv.Close()

	d := webhook.NewAsyncDispatcher(webhook.NewSender(srv.URL, newTestSigner(t)), 6, tinyBackoff())
	d.Emit(webhook.EventOnCloseConnection, map[string]string{"user_id": "u1"})

	require.Eventually(t, func() bool { return len(cs.envelopes()) == 3 }, 2*time.Second, 5*time.Millisecond)
	d.Shutdown(context.Background())

	envs := cs.envelopes()
	for _, e := range envs {
		require.Equal(t, webhook.EventOnCloseConnection, e.Meta.EventType)
		require.Equal(t, envs[0].Meta.CallbackID, e.Meta.CallbackID, "retries must reuse the callback id")
	}
}

func TestAsyncDispatcher_DropsAfterMaxRetries(t *testing.T) {
	cs := &collectServer{failures: 1000}
	srv := httptest.NewServer(cs.handler())
	defer srv.Close()

	d := webhook.NewAsyncDispatcher(webhook.NewSender(srv.URL, newTestSigner(t)), 3, tinyBackoff())
	d.Emit(webhook.EventOnReadError, map[string]string{"user_id": "u1"})

	require.Eventually(t, func() bool { return len(cs.envelopes()) == 3 }, 2*time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond) // would-be 4th attempt window
	d.Shutdown(context.Background())
	require.Len(t, cs.envelopes(), 3, "must stop after retryMax attempts")
}

// TestAsyncDispatcher_Shutdown_ContextExpiryEarlyReturn verifies that
// Shutdown returns as soon as the passed context expires, rather than
// blocking until the in-flight (or queued) deliveries finish draining.
func TestAsyncDispatcher_Shutdown_ContextExpiryEarlyReturn(t *testing.T) {
	block := make(chan struct{})
	var reqStarted int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqStarted, 1)
		<-block // block the delivery so Shutdown can't drain it
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := webhook.NewAsyncDispatcher(webhook.NewSender(srv.URL, newTestSigner(t)), 3, tinyBackoff())
	defer close(block) // release the blocked handler so srv.Close() doesn't hang

	d.Emit(webhook.EventOnCloseConnection, map[string]string{"user_id": "u1"})
	// Give the dispatcher a moment to start delivering (it will be stuck
	// inside sender.Post, blocked on the handler above).
	require.Eventually(t, func() bool { return atomic.LoadInt32(&reqStarted) >= 1 }, time.Second, 2*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	d.Shutdown(ctx)
	elapsed := time.Since(start)

	require.Less(t, elapsed, 1*time.Second,
		"Shutdown must return once ctx expires rather than waiting for the blocked in-flight delivery")
	require.GreaterOrEqual(t, elapsed, 40*time.Millisecond,
		"Shutdown returned suspiciously early, before the ctx timeout could have fired")
}

// TestAsyncDispatcher_Emit_DropsWhenQueueFull verifies that Emit drops
// events (and logs a warning) once the internal queue buffer — the
// unexported asyncQueueSize const (1024) in async.go — is full.
func TestAsyncDispatcher_Emit_DropsWhenQueueFull(t *testing.T) {
	block := make(chan struct{})
	var reqStarted int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqStarted, 1)
		<-block // block the single worker so the queue can fill up
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prevLogger := slog.Default()
	defer slog.SetDefault(prevLogger)

	d := webhook.NewAsyncDispatcher(webhook.NewSender(srv.URL, newTestSigner(t)), 3, tinyBackoff())
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	defer d.Shutdown(shutdownCtx)

	defer close(block) // release the blocked handler so Shutdown/srv.Close() don't hang

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	// The first Emit is picked up immediately by the dispatcher's single
	// worker goroutine and blocks in-flight (stuck in sender.Post), leaving
	// the whole 1024-slot buffer free to fill up.
	d.Emit(webhook.EventOnCloseConnection, map[string]string{"user_id": "u0"})
	require.Eventually(t, func() bool { return atomic.LoadInt32(&reqStarted) >= 1 }, time.Second, 2*time.Millisecond)

	// asyncQueueSize is 1024 (unexported in async.go). Emit 1024 more to
	// fill the buffer exactly, then a couple more to force the `default:`
	// drop branch.
	const asyncQueueSize = 1024
	for i := 0; i < asyncQueueSize+2; i++ {
		d.Emit(webhook.EventOnCloseConnection, map[string]string{"user_id": "u"})
	}

	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "async queue full")
	}, time.Second, 2*time.Millisecond, "expected a queue-full warning to be logged")
}
