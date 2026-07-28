package serverconfig_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	serverconfig "github.com/pipewave-dev/go-pkg/server/config"
	"github.com/stretchr/testify/require"
)

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

const validYAML = `
SERVER:
  API_KEYS: ["key-1", "key-2"]
  AUTH:
    MODE: "webhook"
  CALLBACKS:
    BASE_URL: "https://app.example.com/pipewave/callback"
`

func TestLoad_DefaultsApplied(t *testing.T) {
	cfg, err := serverconfig.Load([]string{writeYAML(t, validYAML)})
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.ClientAddr)
	require.Equal(t, ":8081", cfg.AdminAddr)
	require.Equal(t, serverconfig.RepositoryPostgres, cfg.Repository)
	require.Equal(t, []string{"key-1", "key-2"}, cfg.APIKeys)
	require.Equal(t, serverconfig.HandleMsgModeSync, cfg.Callbacks.HandleMessage.Mode)
	require.Equal(t, 5*time.Second, cfg.Callbacks.HandleMessage.Timeout)
	require.Equal(t, 3*time.Second, cfg.Callbacks.SyncTimeout)
	require.Equal(t, 6, cfg.Callbacks.AsyncRetryMax)
	require.Equal(t, serverconfig.SignatureModeEnabled, cfg.Callbacks.Signature.Mode)
	require.Equal(t, "webhook_ed25519.key", cfg.Callbacks.Signature.SigningKeyFile)
	require.Equal(t, "sub", cfg.Auth.JWT.UserIDClaim)
}

func TestLoad_ExplicitValues(t *testing.T) {
	cfg, err := serverconfig.Load([]string{writeYAML(t, `
SERVER:
  CLIENT_ADDR: ":9090"
  ADMIN_ADDR: ":9091"
  API_KEYS: ["k"]
  REPOSITORY: "dynamodb"
  AUTH:
    MODE: "jwt"
    JWT:
      JWKS_URL: "https://app.example.com/.well-known/jwks.json"
      USER_ID_CLAIM: "uid"
      METADATA_CLAIMS: ["role", "tenant"]
  CALLBACKS:
    BASE_URL: "https://app.example.com/cb"
    HANDLE_MESSAGE:
      MODE: "forward"
      TIMEOUT: "10s"
    SYNC_TIMEOUT: "1s"
    ASYNC_RETRY_MAX: 3
`)})
	require.NoError(t, err)
	require.Equal(t, ":9090", cfg.ClientAddr)
	require.Equal(t, serverconfig.RepositoryDynamoDB, cfg.Repository)
	require.Equal(t, serverconfig.AuthModeJWT, cfg.Auth.Mode)
	require.Equal(t, "uid", cfg.Auth.JWT.UserIDClaim)
	require.Equal(t, []string{"role", "tenant"}, cfg.Auth.JWT.MetadataClaims)
	require.Equal(t, serverconfig.HandleMsgModeForward, cfg.Callbacks.HandleMessage.Mode)
	require.Equal(t, 10*time.Second, cfg.Callbacks.HandleMessage.Timeout)
	require.Equal(t, 1*time.Second, cfg.Callbacks.SyncTimeout)
	require.Equal(t, 3, cfg.Callbacks.AsyncRetryMax)
}

func TestLoad_SignatureDisabled(t *testing.T) {
	cfg, err := serverconfig.Load([]string{writeYAML(t, `
SERVER:
  API_KEYS: ["k"]
  AUTH: {MODE: "webhook"}
  CALLBACKS:
    BASE_URL: "https://x/cb"
    SIGNATURE:
      MODE: "disabled"
      SIGNING_KEY_FILE: "custom.key"
`)})
	require.NoError(t, err)
	require.Equal(t, serverconfig.SignatureModeDisabled, cfg.Callbacks.Signature.Mode)
	require.Equal(t, "custom.key", cfg.Callbacks.Signature.SigningKeyFile)
}

func TestLoad_ValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"missing api keys", `
SERVER:
  AUTH: {MODE: "webhook"}
  CALLBACKS: {BASE_URL: "https://x/cb"}
`, "API_KEYS"},
		{"bad auth mode", `
SERVER:
  API_KEYS: ["k"]
  AUTH: {MODE: "nope"}
  CALLBACKS: {BASE_URL: "https://x/cb"}
`, "AUTH.MODE"},
		{"jwt mode without key source", `
SERVER:
  API_KEYS: ["k"]
  AUTH: {MODE: "jwt"}
  CALLBACKS: {BASE_URL: "https://x/cb"}
`, "JWKS_URL"},
		{"missing base url", `
SERVER:
  API_KEYS: ["k"]
  AUTH: {MODE: "webhook"}
`, "BASE_URL"},
		{"bad handle message mode", `
SERVER:
  API_KEYS: ["k"]
  AUTH: {MODE: "webhook"}
  CALLBACKS:
    BASE_URL: "https://x/cb"
    HANDLE_MESSAGE: {MODE: "async"}
`, "HANDLE_MESSAGE.MODE"},
		{"bad repository", `
SERVER:
  API_KEYS: ["k"]
  REPOSITORY: "mysql"
  AUTH: {MODE: "webhook"}
  CALLBACKS: {BASE_URL: "https://x/cb"}
`, "REPOSITORY"},
		{"bad signature mode", `
SERVER:
  API_KEYS: ["k"]
  AUTH: {MODE: "webhook"}
  CALLBACKS:
    BASE_URL: "https://x/cb"
    SIGNATURE: {MODE: "on"}
`, "SIGNATURE.MODE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := serverconfig.Load([]string{writeYAML(t, tc.yaml)})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestLoad_CallbackResilienceDefaults(t *testing.T) {
	cfg, err := serverconfig.Load([]string{writeYAML(t, validYAML)})
	require.NoError(t, err)
	require.Equal(t, 1, cfg.Callbacks.SyncRetry.Max)                 // no-retry preserved
	require.Equal(t, 100*time.Millisecond, cfg.Callbacks.SyncRetry.Backoff)
	require.Equal(t, 5, cfg.Callbacks.Breaker.Threshold)
	require.Equal(t, 10*time.Second, cfg.Callbacks.Breaker.Cooldown)
	require.False(t, cfg.Callbacks.Ping.Enabled)
	require.Equal(t, serverconfig.UnhealthyActionLogOnly, cfg.Callbacks.UnhealthyAction)
	require.Equal(t, time.Duration(0), cfg.Callbacks.BreakerOpenShutdown)
	require.Empty(t, cfg.Callbacks.AsyncBackoff)
}

func TestLoad_PingDefaultsWhenEnabled(t *testing.T) {
	cfg, err := serverconfig.Load([]string{writeYAML(t, `
SERVER:
  API_KEYS: ["k"]
  AUTH:
    MODE: "webhook"
  CALLBACKS:
    BASE_URL: "https://app.example.com/cb"
    PING:
      ENABLED: true
`)})
	require.NoError(t, err)
	require.True(t, cfg.Callbacks.Ping.Enabled)
	require.Equal(t, "/pipewave/ping", cfg.Callbacks.Ping.Path)
	require.Equal(t, 30*time.Second, cfg.Callbacks.Ping.Interval)
	require.Equal(t, 3*time.Second, cfg.Callbacks.Ping.Timeout)
	require.True(t, cfg.Callbacks.Ping.BootCheck)
	require.Equal(t, 3, cfg.Callbacks.Ping.FailThreshold)
}

func TestLoad_RejectsBadUnhealthyAction(t *testing.T) {
	_, err := serverconfig.Load([]string{writeYAML(t, `
SERVER:
  API_KEYS: ["k"]
  AUTH:
    MODE: "webhook"
  CALLBACKS:
    BASE_URL: "https://app.example.com/cb"
    UNHEALTHY_ACTION: "explode"
`)})
	require.Error(t, err)
}
