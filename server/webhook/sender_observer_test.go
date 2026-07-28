package webhook_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

type call struct {
	eventType  string
	mode       string
	statusCode int
	err        error
}

type spyObserver struct {
	mu      sync.Mutex
	calls   []call
	retries []string
	dropped []string
}

func (s *spyObserver) ObserveCall(eventType, mode string, _ time.Duration, statusCode int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call{eventType, mode, statusCode, err})
}

func (s *spyObserver) ObserveRetry(eventType, mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retries = append(s.retries, eventType+":"+mode)
}

func (s *spyObserver) ObserveDropped(eventType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropped = append(s.dropped, eventType)
}

func TestSender_ObserverSeesSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	spy := &spyObserver{}
	sender := webhook.NewSender(srv.URL, nil)
	sender.SetObserver(spy)

	_, _, err := sender.Post(context.Background(), webhook.EventOnCloseConnection, "cb1", map[string]string{}, time.Second)
	require.NoError(t, err)

	require.Len(t, spy.calls, 1)
	require.Equal(t, webhook.EventOnCloseConnection, spy.calls[0].eventType)
	require.Equal(t, webhook.ModeSync, spy.calls[0].mode)
	require.Equal(t, http.StatusOK, spy.calls[0].statusCode)
	require.NoError(t, spy.calls[0].err)
}

func TestSender_ObserverSeesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	spy := &spyObserver{}
	sender := webhook.NewSender(srv.URL, nil)
	sender.SetObserver(spy)

	_, _, _ = sender.Post(context.Background(), webhook.EventOnCloseConnection, "cb1", map[string]string{}, time.Second)

	require.Len(t, spy.calls, 1)
	require.Equal(t, http.StatusInternalServerError, spy.calls[0].statusCode)
}

func TestSender_ObserverModeAsync(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	spy := &spyObserver{}
	sender := webhook.NewSender(srv.URL, nil)
	sender.SetObserver(spy)

	_, _, err := sender.PostWithMode(context.Background(), webhook.EventOnCloseConnection, "cb1",
		map[string]string{}, time.Second, webhook.ModeAsync)
	require.NoError(t, err)

	require.Len(t, spy.calls, 1)
	require.Equal(t, webhook.ModeAsync, spy.calls[0].mode)
}

// A nil observer is the default and must stay safe — existing callers never set one.
func TestSender_NilObserverIsSafe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	sender := webhook.NewSender(srv.URL, nil)
	require.NotPanics(t, func() {
		_, _, _ = sender.Post(context.Background(), webhook.EventOnCloseConnection, "cb1", map[string]string{}, time.Second)
	})
}
