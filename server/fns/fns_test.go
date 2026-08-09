package serverfns_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/export/types"
	serverconfig "github.com/pipewave-dev/go-pkg/server/config"
	serverfns "github.com/pipewave-dev/go-pkg/server/fns"
	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

// backend is a scripted callback receiver: respond maps event_type to a
// (status, body) answer; every envelope is recorded.
type backend struct {
	mu      sync.Mutex
	got     []webhook.Body
	respond map[string]struct {
		status int
		body   string
	}
}

func (b *backend) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var env webhook.Body
		_ = json.Unmarshal(raw, &env)
		b.mu.Lock()
		b.got = append(b.got, env)
		resp, ok := b.respond[env.Meta.EventType]
		b.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(resp.body))
	}
}

func (b *backend) envelopes() []webhook.Body {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]webhook.Body(nil), b.got...)
}

func newFns(t *testing.T, b *backend, mode string) *types.Fns {
	t.Helper()
	srv := httptest.NewServer(b.handler())
	t.Cleanup(srv.Close)
	signer, err := webhook.LoadOrGenerateSigner(filepath.Join(t.TempDir(), "key"))
	require.NoError(t, err)
	sender := webhook.NewSender(srv.URL, signer)
	async := webhook.NewAsyncDispatcher(sender, 1, []time.Duration{time.Millisecond})
	t.Cleanup(func() { async.Shutdown(context.Background()) })
	syncCaller := webhook.NewSyncCaller(sender, webhook.NewCircuitBreaker(100, time.Minute), 1, 0)
	return serverfns.New(syncCaller, async, serverfns.Config{
		HandleMessageMode:    mode,
		HandleMessageTimeout: time.Second,
		SyncTimeout:          time.Second,
	})
}

// unreachableURL is a private-looking callback address that refuses TCP
// connections immediately, simulating a transport failure. It must never
// leak into a sanitized error string.
const unreachableURL = "http://127.0.0.1:1/pipewave/callback"

// newFnsUnreachable builds fns backed by a callback URL that cannot be
// connected to, so every sync.Call fails with a transport error (which
// wraps the URL).
func newFnsUnreachable(t *testing.T, mode string) *types.Fns {
	t.Helper()
	signer, err := webhook.LoadOrGenerateSigner(filepath.Join(t.TempDir(), "key"))
	require.NoError(t, err)
	sender := webhook.NewSender(unreachableURL, signer)
	async := webhook.NewAsyncDispatcher(sender, 1, []time.Duration{time.Millisecond})
	t.Cleanup(func() { async.Shutdown(context.Background()) })
	syncCaller := webhook.NewSyncCaller(sender, webhook.NewCircuitBreaker(100, time.Minute), 1, 0)
	return serverfns.New(syncCaller, async, serverfns.Config{
		HandleMessageMode:    mode,
		HandleMessageTimeout: 2 * time.Second,
		SyncTimeout:          2 * time.Second,
	})
}

var testAuth = types.WebsocketAuth{UserID: "u1", InstanceID: "i1", Metadata: map[string]string{"k": "v"}}

func TestInspectToken_WebhookMode(t *testing.T) {
	b := &backend{respond: map[string]struct {
		status int
		body   string
	}{
		webhook.EventInspectToken: {200, `{"user_id":"u9","is_anonymous":false,"metadata":{"role":"admin"}}`},
	}}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	userID, anon, md, err := fns.InspectToken(context.Background(), "tok-1", http.Header{"X-Real-Ip": []string{"1.2.3.4"}})
	require.NoError(t, err)
	require.Equal(t, "u9", userID)
	require.False(t, anon)
	require.Equal(t, map[string]string{"role": "admin"}, md)

	env := b.envelopes()[0]
	require.Equal(t, webhook.EventInspectToken, env.Meta.EventType)
	require.JSONEq(t, `{"token":"tok-1","headers":{"X-Real-Ip":["1.2.3.4"]}}`, string(env.Data))
}

func TestInspectToken_FailsClosedOn5xx(t *testing.T) {
	// Body deliberately carries sensitive-looking detail: a 5xx is an
	// infrastructure failure, not a deliberate rejection, so it must never
	// reach the client verbatim.
	rawBody := `internal error: postgres dial tcp 10.0.0.5:5432: connection refused`
	b := &backend{respond: map[string]struct {
		status int
		body   string
	}{webhook.EventInspectToken: {500, rawBody}}}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	_, _, _, err := fns.InspectToken(context.Background(), "tok", nil)
	require.Error(t, err)
	require.Equal(t, "authentication failed", err.Error())
	require.NotContains(t, err.Error(), "10.0.0.5")
	require.NotContains(t, err.Error(), "postgres")
}

// TestInspectToken_EmptyUserIDNonAnonymousFailsClosed covers a 200 response
// that claims a non-anonymous identity but supplies no user_id: core panics
// on an empty userID for non-anonymous auth, so this must fail closed
// instead of returning ("", false, ...).
func TestInspectToken_EmptyUserIDNonAnonymousFailsClosed(t *testing.T) {
	b := &backend{respond: map[string]struct {
		status int
		body   string
	}{webhook.EventInspectToken: {200, `{"user_id":"","is_anonymous":false}`}}}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	userID, anon, _, err := fns.InspectToken(context.Background(), "tok", nil)
	require.Error(t, err)
	require.Empty(t, userID)
	require.False(t, anon)
}

// TestInspectToken_TransportFailureSanitized covers an unreachable backend
// (connection refused): the transport error from net/http embeds the
// callback URL/host, which must never leak into the error surfaced to the
// WS client.
func TestInspectToken_TransportFailureSanitized(t *testing.T) {
	fns := newFnsUnreachable(t, serverconfig.HandleMsgModeSync)

	_, _, _, err := fns.InspectToken(context.Background(), "tok", nil)
	require.Error(t, err)
	require.Equal(t, "authentication failed", err.Error())
	require.NotContains(t, err.Error(), "127.0.0.1")
}

// TestHandleMessage_SyncMode_TransportFailureSanitized mirrors the above
// for the handle_message sync hook, whose error is surfaced as a WS error
// frame straight to the end client.
func TestHandleMessage_SyncMode_TransportFailureSanitized(t *testing.T) {
	fns := newFnsUnreachable(t, serverconfig.HandleMsgModeSync)

	_, _, err := fns.HandleMessage.HandleMessage(context.Background(), testAuth, "X", nil)
	require.Error(t, err)
	require.Equal(t, "upstream error", err.Error())
	require.NotContains(t, err.Error(), "127.0.0.1")
}

// TestHandleMessage_SyncMode_5xxBodySanitized covers a reachable backend
// that returns 500 with a raw body containing internal detail — an
// infrastructure failure, so the raw body must not reach the client.
func TestHandleMessage_SyncMode_5xxBodySanitized(t *testing.T) {
	rawBody := `panic: runtime error at internal/db.go:42, dsn=postgres://admin:hunter2@10.0.0.5/app`
	b := &backend{respond: map[string]struct {
		status int
		body   string
	}{webhook.EventHandleMessage: {500, rawBody}}}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	_, _, err := fns.HandleMessage.HandleMessage(context.Background(), testAuth, "X", nil)
	require.Error(t, err)
	require.Equal(t, "upstream error", err.Error())
	require.NotContains(t, err.Error(), "hunter2")
	require.NotContains(t, err.Error(), "10.0.0.5")
}

func TestHandleMessage_SyncMode(t *testing.T) {
	b := &backend{respond: map[string]struct {
		status int
		body   string
	}{
		webhook.EventHandleMessage: {200, `{"output_type":"ECHO_RESPONSE","data":"aGVsbG8="}`}, // "hello"
	}}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	outType, res, err := fns.HandleMessage.HandleMessage(context.Background(), testAuth, "ECHO", []byte("ping"))
	require.NoError(t, err)
	require.Equal(t, "ECHO_RESPONSE", outType)
	require.Equal(t, []byte("hello"), res)

	env := b.envelopes()[0]
	require.JSONEq(t, `{"auth":{"user_id":"u1","instance_id":"i1","metadata":{"k":"v"}},"input_type":"ECHO","data":"cGluZw=="}`, string(env.Data))
}

func TestHandleMessage_SyncModeErrorFromBackend(t *testing.T) {
	b := &backend{respond: map[string]struct {
		status int
		body   string
	}{webhook.EventHandleMessage: {422, `{"error":"bad msg"}`}}}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	_, _, err := fns.HandleMessage.HandleMessage(context.Background(), testAuth, "X", nil)
	require.Error(t, err)
	// A 4xx is a deliberate backend rejection: the {"error": "..."} message
	// is a deliberate application-level answer, so it is surfaced verbatim.
	require.Equal(t, "bad msg", err.Error())
}

// TestHandleMessage_SyncMode_4xxWithoutErrorField covers a 4xx response
// that doesn't carry a parseable {"error": "..."} body: falls back to a
// generic "<hook> rejected" rather than leaking the raw body.
func TestHandleMessage_SyncMode_4xxWithoutErrorField(t *testing.T) {
	b := &backend{respond: map[string]struct {
		status int
		body   string
	}{webhook.EventHandleMessage: {400, `not json`}}}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	_, _, err := fns.HandleMessage.HandleMessage(context.Background(), testAuth, "X", nil)
	require.Error(t, err)
	require.Equal(t, webhook.EventHandleMessage+" rejected", err.Error())
	require.NotContains(t, err.Error(), "not json")
}

func TestHandleMessage_ForwardMode(t *testing.T) {
	b := &backend{}
	fns := newFns(t, b, serverconfig.HandleMsgModeForward)

	outType, res, err := fns.HandleMessage.HandleMessage(context.Background(), testAuth, "TELEMETRY", []byte("x"))
	require.NoError(t, err)
	require.Empty(t, outType)
	require.Nil(t, res)

	require.Eventually(t, func() bool { return len(b.envelopes()) == 1 }, time.Second, 5*time.Millisecond)
	require.Equal(t, webhook.EventMessageReceived, b.envelopes()[0].Meta.EventType)
}

func TestHandleMessage_DisabledMode(t *testing.T) {
	b := &backend{}
	fns := newFns(t, b, serverconfig.HandleMsgModeDisabled)

	outType, res, err := fns.HandleMessage.HandleMessage(context.Background(), testAuth, "ANY", []byte("x"))
	require.NoError(t, err)
	require.Empty(t, outType)
	require.Nil(t, res)

	time.Sleep(20 * time.Millisecond) // give any (wrong) async emit a chance to land
	require.Empty(t, b.envelopes(), "disabled mode must not call the backend at all")
}

func TestOnNewConnection_AcceptEmitsEstablished(t *testing.T) {
	b := &backend{}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	require.NoError(t, fns.OnNewConnection.OnNewConnection(context.Background(), testAuth))
	require.Eventually(t, func() bool { return len(b.envelopes()) == 2 }, time.Second, 5*time.Millisecond)

	envs := b.envelopes()
	require.Equal(t, webhook.EventOnNewConnection, envs[0].Meta.EventType) // sync, first
	require.Equal(t, webhook.EventOnNewConnectionEstablished, envs[1].Meta.EventType)
}

func TestOnNewConnection_RejectOn4xx(t *testing.T) {
	b := &backend{respond: map[string]struct {
		status int
		body   string
	}{webhook.EventOnNewConnection: {403, `{"error":"banned"}`}}}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	err := fns.OnNewConnection.OnNewConnection(context.Background(), testAuth)
	require.Error(t, err)
	// Deliberate rejection: the backend's message is surfaced verbatim.
	require.Equal(t, "banned", err.Error())
	time.Sleep(20 * time.Millisecond)
	require.Len(t, b.envelopes(), 1, "no established event on reject")
}

func TestAsyncHooks(t *testing.T) {
	b := &backend{}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	fns.OnCloseConnection.OnCloseConnection(context.Background(), testAuth)
	fns.OnReadError.OnReadError(context.Background(), testAuth, io.ErrUnexpectedEOF)
	fns.OnWriteError.OnWriteError(context.Background(), testAuth, io.ErrClosedPipe)

	require.Eventually(t, func() bool { return len(b.envelopes()) == 3 }, time.Second, 5*time.Millisecond)
	seen := map[string]bool{}
	for _, e := range b.envelopes() {
		seen[e.Meta.EventType] = true
	}
	require.True(t, seen[webhook.EventOnCloseConnection])
	require.True(t, seen[webhook.EventOnReadError])
	require.True(t, seen[webhook.EventOnWriteError])
}

// fakeAsync ghi lại các event Class-2 mà không cần HTTP server.
type fakeAsync struct {
	mu   sync.Mutex
	got  []string
	data []any
}

func (f *fakeAsync) Emit(eventType string, data any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, eventType)
	f.data = append(f.data, data)
}
func (f *fakeAsync) Healthcheck() error         { return nil }
func (f *fakeAsync) Shutdown(_ context.Context) {}
func (f *fakeAsync) events() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got...)
}

// Class-2 hooks phải đi qua AsyncTransport, không phụ thuộc HTTP.
func TestAsyncHooksGoThroughTransport(t *testing.T) {
	fake := &fakeAsync{}
	fns := serverfns.New(nil, fake, serverfns.Config{
		HandleMessageMode: serverconfig.HandleMsgModeDisabled,
	})

	auth := types.WebsocketAuth{UserID: "u1", InstanceID: "i1"}
	fns.OnCloseConnection.OnCloseConnection(context.Background(), auth)
	fns.OnReadError.OnReadError(context.Background(), auth, io.EOF)
	fns.OnWriteError.OnWriteError(context.Background(), auth, io.EOF)

	require.Equal(t, []string{
		webhook.EventOnCloseConnection,
		webhook.EventOnReadError,
		webhook.EventOnWriteError,
	}, fake.events())
}
