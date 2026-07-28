package webhook_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

func TestSyncCaller_DecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user_id":"u1","is_anonymous":false}`))
	}))
	defer srv.Close()

	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)), webhook.NewCircuitBreaker(5, 10*time.Second), 1, 0)
	var out struct {
		UserID      string `json:"user_id"`
		IsAnonymous bool   `json:"is_anonymous"`
	}
	require.NoError(t, c.Call(context.Background(), webhook.EventInspectToken, map[string]string{"token": "t"}, time.Second, &out))
	require.Equal(t, "u1", out.UserID)
}

func TestSyncCaller_Non2xxReturnsCallError_NoBreakerTripOn4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()

	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)), webhook.NewCircuitBreaker(2, 10*time.Second), 1, 0)
	for range 5 { // 4xx repeatedly must NOT open the breaker
		err := c.Call(context.Background(), webhook.EventOnNewConnection, nil, time.Second, nil)
		var ce *webhook.CallError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, http.StatusForbidden, ce.Status)
		require.NotErrorIs(t, err, webhook.ErrCircuitOpen)
	}
}

func TestSyncCaller_BreakerOpensOn5xxAndRecovers(t *testing.T) {
	var healthy atomic.Bool
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if !healthy.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	breaker := webhook.NewCircuitBreaker(2, 30*time.Millisecond)
	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)), breaker, 1, 0)

	// two 5xx failures open the breaker
	for range 2 {
		require.Error(t, c.Call(context.Background(), webhook.EventInspectToken, nil, time.Second, nil))
	}
	// while open: fast-fail without hitting the backend
	before := hits.Load()
	require.ErrorIs(t, c.Call(context.Background(), webhook.EventInspectToken, nil, time.Second, nil), webhook.ErrCircuitOpen)
	require.Equal(t, before, hits.Load())

	// after cooldown, a successful probe closes it again
	healthy.Store(true)
	time.Sleep(40 * time.Millisecond)
	require.NoError(t, c.Call(context.Background(), webhook.EventInspectToken, nil, time.Second, nil))
	require.NoError(t, c.Call(context.Background(), webhook.EventInspectToken, nil, time.Second, nil))
}

func TestSyncCaller_TransportErrorCountsAsFailure(t *testing.T) {
	c := webhook.NewSyncCaller(webhook.NewSender("http://127.0.0.1:1", newTestSigner(t)), webhook.NewCircuitBreaker(1, time.Minute), 1, 0)
	err := c.Call(context.Background(), webhook.EventInspectToken, nil, 50*time.Millisecond, nil)
	require.Error(t, err)
	require.False(t, errors.Is(err, webhook.ErrCircuitOpen))
	// breaker is now open
	require.ErrorIs(t, c.Call(context.Background(), webhook.EventInspectToken, nil, time.Second, nil), webhook.ErrCircuitOpen)
}

func TestSyncCaller_Retries5xxThenSucceeds(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)),
		webhook.NewCircuitBreaker(10, time.Minute), 3, time.Millisecond)
	require.NoError(t, c.Call(context.Background(), webhook.EventInspectToken, nil, time.Second, nil))
	require.Equal(t, int64(2), hits.Load()) // 1 fail + 1 success
}

func TestSyncCaller_Does_Not_Retry_4xx(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()

	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)),
		webhook.NewCircuitBreaker(10, time.Minute), 3, time.Millisecond)
	var ce *webhook.CallError
	require.ErrorAs(t, c.Call(context.Background(), webhook.EventOnNewConnection, nil, time.Second, nil), &ce)
	require.Equal(t, int64(1), hits.Load()) // 4xx = câu trả lời chủ ý, không retry
}

func TestSyncCaller_ReusesCallbackIDAcrossRetries(t *testing.T) {
	var ids sync.Map
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Meta struct {
				CallbackID string `json:"id"`
			} `json:"meta"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		ids.Store(body.Meta.CallbackID, true)
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)),
		webhook.NewCircuitBreaker(10, time.Minute), 3, time.Millisecond)
	require.NoError(t, c.Call(context.Background(), webhook.EventInspectToken, nil, time.Second, nil))
	count := 0
	ids.Range(func(_, _ any) bool { count++; return true })
	require.Equal(t, 1, count) // cùng callbackID cho cả 2 attempt
}

func TestCircuitBreaker_OpenSince(t *testing.T) {
	b := webhook.NewCircuitBreaker(2, time.Minute)
	_, ok := b.OpenSince()
	require.False(t, ok)
	b.Record(false)
	b.Record(false)
	at, ok := b.OpenSince()
	require.True(t, ok)
	require.False(t, at.IsZero())
	b.Record(true) // success closes
	_, ok = b.OpenSince()
	require.False(t, ok)
}
