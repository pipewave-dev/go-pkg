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
	syncCaller := webhook.NewSyncCaller(sender, webhook.NewCircuitBreaker(100, time.Minute))
	return serverfns.New(syncCaller, async, serverfns.Config{
		HandleMessageMode:    mode,
		HandleMessageTimeout: time.Second,
		SyncTimeout:          time.Second,
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
	b := &backend{respond: map[string]struct {
		status int
		body   string
	}{webhook.EventInspectToken: {500, `{}`}}}
	fns := newFns(t, b, serverconfig.HandleMsgModeSync)

	_, _, _, err := fns.InspectToken(context.Background(), "tok", nil)
	require.Error(t, err)
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

	require.Error(t, fns.OnNewConnection.OnNewConnection(context.Background(), testAuth))
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
