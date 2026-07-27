package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	business "github.com/pipewave-dev/go-pkg/core/service/business"
	"github.com/pipewave-dev/go-pkg/server/restapi"
	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/stretchr/testify/require"
)

// fakeServices records calls; unimplemented methods panic via the embedded
// nil interface, which is fine — tests only exercise what they stub.
type fakeServices struct {
	delivery.ExportedServices
	sendToSession        func(ctx context.Context, userID, instanceID, msgType string, payload []byte) aerror.AError
	sendToSessionWithAck func(ctx context.Context, userID, instanceID, msgType string, payload []byte, timeout time.Duration) (bool, aerror.AError)
	sendToUser           func(ctx context.Context, userID, msgType string, payload []byte) aerror.AError
	sendToUsers          func(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError
	sendToAll            func(ctx context.Context, msgType string, payload []byte) aerror.AError
	sendToAnonymous      func(ctx context.Context, msgType string, payload []byte, isSendAll bool, instanceID []string) aerror.AError
	sendToAuthenticated  func(ctx context.Context, msgType string, payload []byte) aerror.AError
	disconnectSession    func(ctx context.Context, userID, instanceID string) aerror.AError
	disconnectUser       func(ctx context.Context, userID string) aerror.AError
	checkOnline          func(ctx context.Context, userID string) (bool, aerror.AError)
	checkOnlineMultiple  func(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError)
	getUserSessions      func(ctx context.Context, userID string) ([]delivery.SessionInfo, aerror.AError)
	cleanUp              func(ctx context.Context) aerror.AError
}

func (f *fakeServices) SendToSession(ctx context.Context, u, i, m string, p []byte) aerror.AError {
	return f.sendToSession(ctx, u, i, m, p)
}
func (f *fakeServices) SendToSessionWithAck(ctx context.Context, u, i, m string, p []byte, t time.Duration) (bool, aerror.AError) {
	return f.sendToSessionWithAck(ctx, u, i, m, p, t)
}
func (f *fakeServices) SendToUser(ctx context.Context, u, m string, p []byte) aerror.AError {
	return f.sendToUser(ctx, u, m, p)
}
func (f *fakeServices) SendToUsers(ctx context.Context, us []string, m string, p []byte) aerror.AError {
	return f.sendToUsers(ctx, us, m, p)
}
func (f *fakeServices) SendToAll(ctx context.Context, m string, p []byte) aerror.AError {
	return f.sendToAll(ctx, m, p)
}
func (f *fakeServices) SendToAnonymous(ctx context.Context, m string, p []byte, all bool, ids []string) aerror.AError {
	return f.sendToAnonymous(ctx, m, p, all, ids)
}
func (f *fakeServices) SendToAuthenticated(ctx context.Context, m string, p []byte) aerror.AError {
	return f.sendToAuthenticated(ctx, m, p)
}
func (f *fakeServices) DisconnectSession(ctx context.Context, u, i string) aerror.AError {
	return f.disconnectSession(ctx, u, i)
}
func (f *fakeServices) DisconnectUser(ctx context.Context, u string) aerror.AError {
	return f.disconnectUser(ctx, u)
}
func (f *fakeServices) CheckOnline(ctx context.Context, u string) (bool, aerror.AError) {
	return f.checkOnline(ctx, u)
}
func (f *fakeServices) CheckOnlineMultiple(ctx context.Context, us []string) (map[string]bool, aerror.AError) {
	return f.checkOnlineMultiple(ctx, us)
}
func (f *fakeServices) GetUserSessions(ctx context.Context, u string) ([]delivery.SessionInfo, aerror.AError) {
	return f.getUserSessions(ctx, u)
}
func (f *fakeServices) CleanUp(ctx context.Context) aerror.AError { return f.cleanUp(ctx) }

type fakeMonitoring struct {
	business.Monitoring
	inside func(ctx context.Context) (*business.SumaryActiveConnection, aerror.AError)
	total  func(ctx context.Context) (int, aerror.AError)
	pool   func(ctx context.Context) (business.WorkerPoolSummary, aerror.AError)
}

func (f *fakeMonitoring) InsideActiveConnection(ctx context.Context) (*business.SumaryActiveConnection, aerror.AError) {
	return f.inside(ctx)
}
func (f *fakeMonitoring) TotalActiveConnection(ctx context.Context) (int, aerror.AError) {
	return f.total(ctx)
}
func (f *fakeMonitoring) WorkerPoolStats(ctx context.Context) (business.WorkerPoolSummary, aerror.AError) {
	return f.pool(ctx)
}

type fakeModule struct {
	delivery.ModuleDelivery
	svc     delivery.ExportedServices
	mon     business.Monitoring
	healthy bool
}

func (f *fakeModule) Services() delivery.ExportedServices { return f.svc }
func (f *fakeModule) Monitoring() business.Monitoring     { return f.mon }
func (f *fakeModule) IsHealthy() bool                     { return f.healthy }

const testKey = "test-api-key"

func newTestMux(svc delivery.ExportedServices, mon business.Monitoring) *httptest.Server {
	mux := restapi.NewAdminMux(&fakeModule{svc: svc, mon: mon, healthy: true}, restapi.MuxConfig{
		APIKeys:   []string{testKey},
		PublicKey: webhook.PublicKeyVerifier{Alg: "Ed25519", PublicKeyInBase64: "cHVi"},
	})
	return httptest.NewServer(mux)
}

func doReq(t *testing.T, method, url, key string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, err := http.NewRequest(method, url, &buf)
	require.NoError(t, err)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestAuth(t *testing.T) {
	srv := newTestMux(&fakeServices{}, &fakeMonitoring{})
	defer srv.Close()

	resp, _ := doReq(t, "GET", srv.URL+"/api/v1/webhook/public-key", "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp, _ = doReq(t, "GET", srv.URL+"/api/v1/webhook/public-key", "wrong", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp, out := doReq(t, "GET", srv.URL+"/api/v1/webhook/public-key", testKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "Ed25519", out["alg"])
	require.Equal(t, "cHVi", out["public_key_in_base64"])

	// healthz needs no key
	resp, out = doReq(t, "GET", srv.URL+"/healthz", "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, true, out["healthy"])

	// raw key without Bearer prefix must fail
	req, err := http.NewRequest("GET", srv.URL+"/api/v1/webhook/public-key", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", testKey)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Bearer without space must fail
	req, err = http.NewRequest("GET", srv.URL+"/api/v1/webhook/public-key", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer"+testKey)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestSendToSession(t *testing.T) {
	var gotUser, gotInstance, gotType string
	var gotPayload []byte
	svc := &fakeServices{
		sendToSession: func(_ context.Context, u, i, m string, p []byte) aerror.AError {
			gotUser, gotInstance, gotType, gotPayload = u, i, m, p
			return nil
		},
		sendToSessionWithAck: func(_ context.Context, u, i, m string, p []byte, timeout time.Duration) (bool, aerror.AError) {
			require.Equal(t, 1500*time.Millisecond, timeout)
			return true, nil
		},
	}
	srv := newTestMux(svc, &fakeMonitoring{})
	defer srv.Close()

	resp, out := doReq(t, "POST", srv.URL+"/api/v1/messages/session", testKey, map[string]any{
		"user_id": "u1", "instance_id": "i1", "msg_type": "GREET", "payload": []byte("hi"),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, true, out["sent"])
	require.Equal(t, "u1", gotUser)
	require.Equal(t, "i1", gotInstance)
	require.Equal(t, "GREET", gotType)
	require.Equal(t, []byte("hi"), gotPayload)

	// ack variant
	resp, out = doReq(t, "POST", srv.URL+"/api/v1/messages/session", testKey, map[string]any{
		"user_id": "u1", "instance_id": "i1", "msg_type": "GREET", "payload": []byte("hi"), "ack_timeout_ms": 1500,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, true, out["acked"])

	// validation: missing user_id
	resp, _ = doReq(t, "POST", srv.URL+"/api/v1/messages/session", testKey, map[string]any{
		"instance_id": "i1", "msg_type": "GREET",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBroadcastTargets(t *testing.T) {
	var called string
	var gotAnonAll bool
	svc := &fakeServices{
		sendToAll:           func(context.Context, string, []byte) aerror.AError { called = "all"; return nil },
		sendToAuthenticated: func(context.Context, string, []byte) aerror.AError { called = "authenticated"; return nil },
		sendToAnonymous: func(_ context.Context, _ string, _ []byte, all bool, _ []string) aerror.AError {
			called, gotAnonAll = "anonymous", all
			return nil
		},
	}
	srv := newTestMux(svc, &fakeMonitoring{})
	defer srv.Close()

	for _, target := range []string{"all", "authenticated", "anonymous"} {
		resp, _ := doReq(t, "POST", srv.URL+"/api/v1/messages/broadcast", testKey, map[string]any{
			"target": target, "msg_type": "NEWS", "payload": []byte("x"),
		})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, target, called)
	}
	require.True(t, gotAnonAll, "no instance_ids means send-all")

	resp, _ := doReq(t, "POST", srv.URL+"/api/v1/messages/broadcast", testKey, map[string]any{
		"target": "nobody", "msg_type": "NEWS",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPresenceAndSessions(t *testing.T) {
	connectedAt := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	svc := &fakeServices{
		checkOnline: func(_ context.Context, u string) (bool, aerror.AError) { return u == "online-user", nil },
		checkOnlineMultiple: func(_ context.Context, us []string) (map[string]bool, aerror.AError) {
			return map[string]bool{"a": true, "b": false}, nil
		},
		getUserSessions: func(_ context.Context, u string) ([]delivery.SessionInfo, aerror.AError) {
			return []delivery.SessionInfo{{UserID: u, InstanceID: "i1", HolderID: "h1", ConnectedAt: connectedAt}}, nil
		},
	}
	srv := newTestMux(svc, &fakeMonitoring{})
	defer srv.Close()

	resp, out := doReq(t, "GET", srv.URL+"/api/v1/presence/online-user", testKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, true, out["online"])

	resp, out = doReq(t, "POST", srv.URL+"/api/v1/presence/batch", testKey, map[string]any{"user_ids": []string{"a", "b"}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, map[string]any{"a": true, "b": false}, out["results"])

	resp, out = doReq(t, "GET", srv.URL+"/api/v1/sessions/u1", testKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	sessions := out["sessions"].([]any)
	require.Len(t, sessions, 1)
	first := sessions[0].(map[string]any)
	require.Equal(t, "i1", first["instance_id"])
	require.Equal(t, "h1", first["holder_id"])
}

func TestDisconnectAndCleanup(t *testing.T) {
	var disconnectedSession, disconnectedUser, cleaned bool
	svc := &fakeServices{
		disconnectSession: func(_ context.Context, u, i string) aerror.AError {
			require.Equal(t, "u1", u)
			require.Equal(t, "i1", i)
			disconnectedSession = true
			return nil
		},
		disconnectUser: func(_ context.Context, u string) aerror.AError { disconnectedUser = true; return nil },
		cleanUp:        func(context.Context) aerror.AError { cleaned = true; return nil },
	}
	srv := newTestMux(svc, &fakeMonitoring{})
	defer srv.Close()

	resp, _ := doReq(t, "DELETE", srv.URL+"/api/v1/sessions/u1/i1", testKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, disconnectedSession)

	resp, _ = doReq(t, "DELETE", srv.URL+"/api/v1/sessions/u1", testKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, disconnectedUser)

	resp, _ = doReq(t, "POST", srv.URL+"/api/v1/maintenance/cleanup", testKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, cleaned)
}

func TestMonitoring(t *testing.T) {
	mon := &fakeMonitoring{
		inside: func(context.Context) (*business.SumaryActiveConnection, aerror.AError) {
			return &business.SumaryActiveConnection{AnonymosConnection: 1, UserConnection: 2, TotalUser: 3}, nil
		},
		total: func(context.Context) (int, aerror.AError) { return 42, nil },
		pool: func(context.Context) (business.WorkerPoolSummary, aerror.AError) {
			return business.WorkerPoolSummary{Length: 5, Capacity: 100, Dropped: 7}, nil
		},
	}
	srv := newTestMux(&fakeServices{}, mon)
	defer srv.Close()

	resp, out := doReq(t, "GET", srv.URL+"/api/v1/monitoring/connections", testKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, float64(42), out["total"])
	inside := out["inside"].(map[string]any)
	require.Equal(t, float64(1), inside["anonymous_connections"])
	require.Equal(t, float64(2), inside["user_connections"])
	require.Equal(t, float64(3), inside["total_users"])

	resp, out = doReq(t, "GET", srv.URL+"/api/v1/monitoring/worker-pool", testKey, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, float64(5), out["length"])
	require.Equal(t, float64(100), out["capacity"])
	require.Equal(t, float64(7), out["dropped"])
}

func TestAErrorMapping(t *testing.T) {
	svc := &fakeServices{
		checkOnline: func(ctx context.Context, u string) (bool, aerror.AError) {
			return false, aerror.New(ctx, aerror.RecordNotFound, nil)
		},
	}
	srv := newTestMux(svc, &fakeMonitoring{})
	defer srv.Close()

	resp, out := doReq(t, "GET", srv.URL+"/api/v1/presence/ghost", testKey, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	errObj := out["error"].(map[string]any)
	require.Equal(t, "RecordNotFound", errObj["code"])
}

func TestWebhookPublicKeyDisabled(t *testing.T) {
	mux := restapi.NewAdminMux(&fakeModule{svc: &fakeServices{}, mon: &fakeMonitoring{}, healthy: true}, restapi.MuxConfig{
		APIKeys: []string{testKey},
		// PublicKey left zero-value → signature disabled
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, out := doReq(t, "GET", srv.URL+"/api/v1/webhook/public-key", testKey, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.Equal(t, "webhook signature is disabled", out["error"])
}
