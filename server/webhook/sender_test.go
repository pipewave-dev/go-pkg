package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

func newTestSigner(t *testing.T) *webhook.Signer {
	t.Helper()
	s, err := webhook.LoadOrGenerateSigner(filepath.Join(t.TempDir(), "key"))
	require.NoError(t, err)
	return s
}

func TestSender_PostSignedEnvelope(t *testing.T) {
	signer := newTestSigner(t)

	var gotBody []byte
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get(webhook.SignatureHeader)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"pong":true}`))
	}))
	defer srv.Close()

	sender := webhook.NewSender(srv.URL, signer)
	status, resp, err := sender.Post(context.Background(), webhook.EventOnCloseConnection, "cb_test1", map[string]string{"user_id": "u1"}, time.Second)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.JSONEq(t, `{"pong":true}`, string(resp))

	// signature verifies over the exact raw body
	require.True(t, signer.Verify(gotBody, gotSig))

	var envelope webhook.Body
	require.NoError(t, json.Unmarshal(gotBody, &envelope))
	require.Equal(t, webhook.EventOnCloseConnection, envelope.Meta.EventType)
	require.Equal(t, "cb_test1", envelope.Meta.CallbackID)
	require.InDelta(t, time.Now().UnixMilli(), envelope.Meta.SentAt, 5000)
	require.JSONEq(t, `{"user_id":"u1"}`, string(envelope.Data))
}

func TestSender_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	sender := webhook.NewSender(srv.URL, newTestSigner(t))
	_, _, err := sender.Post(context.Background(), webhook.EventOnCloseConnection, "cb_t", nil, 20*time.Millisecond)
	require.Error(t, err)
}
