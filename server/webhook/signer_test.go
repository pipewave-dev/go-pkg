package webhook_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

func TestSigner_SignVerifyRoundtrip(t *testing.T) {
	s, err := webhook.LoadOrGenerateSigner(filepath.Join(t.TempDir(), "key"))
	require.NoError(t, err)

	body := []byte(`{"data":{"x":1},"meta":{"sent_at":1,"id":"cb_a","event_type":"on_close_connection"}}`)
	sig := s.Sign(body)
	require.NotEmpty(t, sig)
	require.True(t, s.Verify(body, sig))
	require.False(t, s.Verify([]byte(`tampered`), sig))
	require.False(t, s.Verify(body, "not-base64!!"))
}

func TestSigner_PersistsAcrossLoads(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "key")
	s1, err := webhook.LoadOrGenerateSigner(keyFile)
	require.NoError(t, err)
	s2, err := webhook.LoadOrGenerateSigner(keyFile)
	require.NoError(t, err)

	pk1, pk2 := s1.PublicKey(), s2.PublicKey()
	require.Equal(t, "Ed25519", pk1.Alg)
	require.NotEmpty(t, pk1.PublicKeyInBase64)
	require.Equal(t, pk1, pk2)

	// signatures from the first signer verify with the reloaded one
	body := []byte("hello")
	require.True(t, s2.Verify(body, s1.Sign(body)))
}

func TestNewCallbackID(t *testing.T) {
	id1, id2 := webhook.NewCallbackID(), webhook.NewCallbackID()
	require.True(t, strings.HasPrefix(id1, "cb_"))
	require.NotEqual(t, id1, id2)
}
