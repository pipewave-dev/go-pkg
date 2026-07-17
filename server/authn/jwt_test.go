package authn_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pipewave-dev/go-pkg/server/authn"
	"github.com/stretchr/testify/require"
)

func setupKeys(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	pemFile := filepath.Join(t.TempDir(), "pub.pem")
	require.NoError(t, os.WriteFile(pemFile, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o600))
	return priv, pemFile
}

func signToken(t *testing.T, priv ed25519.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(priv)
	require.NoError(t, err)
	return s
}

func TestJWTInspector_ValidToken(t *testing.T) {
	priv, pemFile := setupKeys(t)
	insp, err := authn.NewJWTInspector(context.Background(), authn.JWTConfig{
		PublicKeyPEMFile: pemFile,
		UserIDClaim:      "sub",
		MetadataClaims:   []string{"role", "tenant"},
	})
	require.NoError(t, err)

	token := signToken(t, priv, jwt.MapClaims{
		"sub":  "user-42",
		"role": "admin",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	userID, anon, md, err := insp.InspectToken(context.Background(), "Bearer "+token, nil)
	require.NoError(t, err)
	require.Equal(t, "user-42", userID)
	require.False(t, anon)
	require.Equal(t, map[string]string{"role": "admin"}, md) // absent "tenant" claim skipped
}

func TestJWTInspector_Rejections(t *testing.T) {
	priv, pemFile := setupKeys(t)
	insp, err := authn.NewJWTInspector(context.Background(), authn.JWTConfig{PublicKeyPEMFile: pemFile, UserIDClaim: "sub"})
	require.NoError(t, err)

	t.Run("expired", func(t *testing.T) {
		token := signToken(t, priv, jwt.MapClaims{"sub": "u", "exp": time.Now().Add(-time.Hour).Unix()})
		_, _, _, err := insp.InspectToken(context.Background(), token, nil)
		require.Error(t, err)
	})
	t.Run("missing exp", func(t *testing.T) {
		token := signToken(t, priv, jwt.MapClaims{"sub": "u"})
		_, _, _, err := insp.InspectToken(context.Background(), token, nil)
		require.Error(t, err)
	})
	t.Run("missing user id claim", func(t *testing.T) {
		token := signToken(t, priv, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()})
		_, _, _, err := insp.InspectToken(context.Background(), token, nil)
		require.Error(t, err)
	})
	t.Run("wrong key", func(t *testing.T) {
		otherPriv, _ := setupKeys(t)
		token := signToken(t, otherPriv, jwt.MapClaims{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()})
		_, _, _, err := insp.InspectToken(context.Background(), token, nil)
		require.Error(t, err)
	})
	t.Run("garbage", func(t *testing.T) {
		_, _, _, err := insp.InspectToken(context.Background(), "not-a-jwt", nil)
		require.Error(t, err)
	})
	t.Run("HS256 algorithm confusion", func(t *testing.T) {
		pemBytes, err := os.ReadFile(pemFile)
		require.NoError(t, err)
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": "u",
			"exp": time.Now().Add(time.Hour).Unix(),
		}).SignedString(pemBytes)
		require.NoError(t, err)
		_, _, _, err = insp.InspectToken(context.Background(), token, nil)
		require.Error(t, err)
	})
	t.Run("alg none", func(t *testing.T) {
		token, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
			"sub": "u",
			"exp": time.Now().Add(time.Hour).Unix(),
		}).SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)
		_, _, _, err = insp.InspectToken(context.Background(), token, nil)
		require.Error(t, err)
	})
}
