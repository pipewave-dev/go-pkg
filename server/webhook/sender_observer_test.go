package webhook_test

import (
	"context"
	"net"
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

// durationSpy is a minimal, single-purpose observer that additionally
// records the reported duration — spyObserver above deliberately discards
// it, so this stays local to the one test that needs it rather than
// reshaping the shared spy.
type durationSpy struct {
	mu  sync.Mutex
	dur time.Duration
	saw bool
}

func (d *durationSpy) ObserveCall(_, _ string, dur time.Duration, _ int, _ error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dur = dur
	d.saw = true
}
func (d *durationSpy) ObserveRetry(_, _ string) {}
func (d *durationSpy) ObserveDropped(_ string)  {}

// TestSender_TruncatedBodyReturnsNilBody pins the fix for a regression found
// in review: PostWithMode's io.ReadAll error branch used to fall through to
// `return status, body, err` where `body` (a named return) had already been
// partially filled by the failed ReadAll, so a mid-body read failure leaked
// truncated bytes instead of nil. It is currently inert (no caller reads
// body when err != nil) but the contract matters for future callers.
//
// A raw TCP listener is used (rather than httptest.Server) because the
// failure must happen AFTER the client has parsed a valid status line and
// headers -- an httptest handler that aborts the handler (e.g. panicking
// with http.ErrAbortHandler) severs the connection too early and instead
// surfaces as an error from httpClient.Do itself (status stays 0), which is
// a different code path than the one being pinned here.
func TestSender_TruncatedBodyReturnsNilBody(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		_, _ = conn.Read(buf) // drain the request; content doesn't matter
		// A Content-Length far larger than the bytes actually written, then
		// closing the connection, makes io.ReadAll fail with an
		// unexpected-EOF after status/headers are already parsed.
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1000000\r\n\r\nshort"))
	}()

	spy := &durationSpy{}
	sender := webhook.NewSender("http://"+ln.Addr().String(), nil)
	sender.SetObserver(spy)

	status, body, callErr := sender.Post(context.Background(), webhook.EventOnCloseConnection, "cb1", map[string]string{}, time.Second)

	require.Equal(t, http.StatusOK, status, "status line was received before the body read failed")
	require.Error(t, callErr)
	require.Nil(t, body, "a failed body read must not leak partially-read bytes")

	spy.mu.Lock()
	defer spy.mu.Unlock()
	require.True(t, spy.saw, "observer must still be invoked on this error path")
	require.Greater(t, spy.dur, time.Duration(0), "duration must still be reported even though body is nil")
}
