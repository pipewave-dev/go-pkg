package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
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
