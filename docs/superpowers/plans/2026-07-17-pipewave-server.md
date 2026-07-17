# Pipewave Server (REST API + Webhook Container) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship pipewave as a standalone container: `ExportedServices`/`Monitoring` exposed as a REST admin API, and the injected `types.Fns` hooks replaced by signed HTTP callbacks (sync invocations + async webhooks) to an app backend.

**Architecture:** A new `server/` package tree wraps the existing `delivery.ModuleDelivery` without touching `core/`. `server/webhook` implements the signed envelope, Ed25519 signer, HTTP sender, async retry dispatcher, and sync caller with circuit breaker. `server/fns` adapts those into a `*types.Fns`. `server/restapi` builds the admin `*http.ServeMux` (also usable by Go embedders). `cmd/pipewave-server` wires config → adapters → listeners.

**Tech Stack:** Go 1.25, stdlib `net/http` (Go 1.22+ method/path patterns), `crypto/ed25519`, koanf (via existing `pkg/koanf`), testify, `github.com/golang-jwt/jwt/v5` + `github.com/MicahParks/keyfunc/v3` (new deps, Task 6 only).

**Spec:** `docs/feats/2026-07-17-rest-api-container-design.md` — read it before starting any task.

## Global Constraints

- Module path: `github.com/pipewave-dev/go-pkg`. Go `1.25.5`.
- **No changes to `core/`, `provider/`, `export/`, `shared/`** — the server is strictly a wrapper (spec non-goal).
- Errors surfaced over REST use `shared/aerror`: HTTP status from `ErrorCode.HttpCode()`, body `{"error":{"code":"<ErrorCode.String()>","message":"<Error()>"}}`.
- Webhook signature header name: `X-Pipewave-Signature` (base64 Ed25519 signature over the raw request body).
- Callback envelope JSON: `{"data":{...},"meta":{"sent_at":<unix ms>,"id":"cb_...","event_type":"..."}}`.
- All callbacks (sync and async) POST to the single configured `SERVER.CALLBACKS.BASE_URL`; receivers switch on `meta.event_type`.
- Config lives under a `SERVER:` key in the same YAML files the core reads; koanf tags are UPPER_SNAKE; env override prefix `APP` (nested via `__`, e.g. `APP_SERVER__ADMIN_ADDR`).
- Defaults: `CLIENT_ADDR=":8080"`, `ADMIN_ADDR=":8081"`, `SYNC_TIMEOUT=3s`, `HANDLE_MESSAGE.TIMEOUT=5s`, `HANDLE_MESSAGE.MODE="sync"`, `ASYNC_RETRY_MAX=6`, `AUTH.JWT.USER_ID_CLAIM="sub"`, `REPOSITORY="postgres"`, `SIGNING_KEY_FILE="webhook_ed25519.key"`.
- Async backoff schedule: `1s, 5s, 30s, 2m, 10m` (last value repeats). Loss on crash is accepted (decided in spec).
- Circuit breaker: opens after 5 consecutive transport/5xx failures, cooldown 10s. 4xx responses are app-level answers, NOT breaker failures.
- Sync callback failure policy: `inspect_token` and `on_new_connection` fail **closed** (reject); `handle_message` returns an error to the WS client.
- Tests use `github.com/stretchr/testify/require`. Run any test with `go test ./server/... -run <Name> -v`.
- Commit format: `feat(server): ...` / `test(server): ...` (repo history is unstructured; use these going forward).
- **Deferred from v1** (marked in the spec, do NOT implement): single-port mode (embedders get it via `NewAdminMux`; the container always uses two listeners) and a Prometheus `/metrics` endpoint (drops are observable via warning logs for now).

---

### Task 1: Server config (`server/config`)

**Files:**
- Create: `server/config/config.go`
- Test: `server/config/config_test.go`

**Interfaces:**
- Consumes: `pkg/koanf` (`koanfpvd.NewKoanfProvider`, `Unmarshall`), pattern reference: `provider/config-provider/1.from_yaml.go`.
- Produces (used by Tasks 7, 9):
  - `serverconfig.Load(yamlFiles []string) (*ServerConfigT, error)`
  - `type ServerConfigT struct { ClientAddr, AdminAddr string; APIKeys []string; Repository string; Auth AuthT; Callbacks CallbacksT }`
  - `type AuthT struct { Mode string; JWT JWTT }`, `type JWTT struct { JWKSURL, PublicKeyPEMFile, UserIDClaim string; MetadataClaims []string }`
  - `type CallbacksT struct { BaseURL, SigningKeyFile string; HandleMessage HandleMsgT; SyncTimeout time.Duration; AsyncRetryMax int }`, `type HandleMsgT struct { Mode string; Timeout time.Duration }`
  - Constants: `AuthModeJWT="jwt"`, `AuthModeWebhook="webhook"`, `HandleMsgModeSync="sync"`, `HandleMsgModeForward="forward"`, `HandleMsgModeDisabled="disabled"`, `RepositoryPostgres="postgres"`, `RepositoryDynamoDB="dynamodb"`

- [ ] **Step 1: Write the failing test**

```go
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
	require.Equal(t, "webhook_ed25519.key", cfg.Callbacks.SigningKeyFile)
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := serverconfig.Load([]string{writeYAML(t, tc.yaml)})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/config/... -v`
Expected: FAIL — `no required module provides package .../server/config` (package does not exist yet).

- [ ] **Step 3: Write the implementation**

```go
// server/config/config.go
package serverconfig

import (
	"fmt"
	"time"

	koanfpvd "github.com/pipewave-dev/go-pkg/pkg/koanf"
)

const (
	AuthModeJWT     = "jwt"
	AuthModeWebhook = "webhook"

	HandleMsgModeSync     = "sync"
	HandleMsgModeForward  = "forward"
	HandleMsgModeDisabled = "disabled"

	RepositoryPostgres = "postgres"
	RepositoryDynamoDB = "dynamodb"
)

type ServerConfigT struct {
	ClientAddr string     `koanf:"CLIENT_ADDR"`
	AdminAddr  string     `koanf:"ADMIN_ADDR"`
	APIKeys    []string   `koanf:"API_KEYS"`
	Repository string     `koanf:"REPOSITORY"`
	Auth       AuthT      `koanf:"AUTH"`
	Callbacks  CallbacksT `koanf:"CALLBACKS"`
}

type AuthT struct {
	Mode string `koanf:"MODE"` // jwt | webhook
	JWT  JWTT   `koanf:"JWT"`
}

type JWTT struct {
	JWKSURL          string   `koanf:"JWKS_URL"`
	PublicKeyPEMFile string   `koanf:"PUBLIC_KEY_PEM_FILE"`
	UserIDClaim      string   `koanf:"USER_ID_CLAIM"`
	MetadataClaims   []string `koanf:"METADATA_CLAIMS"`
}

type CallbacksT struct {
	BaseURL        string        `koanf:"BASE_URL"`
	SigningKeyFile string        `koanf:"SIGNING_KEY_FILE"`
	HandleMessage  HandleMsgT    `koanf:"HANDLE_MESSAGE"`
	SyncTimeout    time.Duration `koanf:"SYNC_TIMEOUT"`
	AsyncRetryMax  int           `koanf:"ASYNC_RETRY_MAX"`
}

type HandleMsgT struct {
	Mode    string        `koanf:"MODE"` // sync | forward | disabled
	Timeout time.Duration `koanf:"TIMEOUT"`
}

type rootT struct {
	Server ServerConfigT `koanf:"SERVER"`
}

// Load reads the SERVER section from the given YAML files (later files
// override earlier ones), merges APP_-prefixed env vars on top, applies
// defaults, and validates. It intentionally reuses pkg/koanf so the server
// section lives in the same files as the core EnvType config.
func Load(yamlFiles []string) (*ServerConfigT, error) {
	koanfYamlFiles := make([]struct {
		FileDir   string
		FilePath  string
		SkipError bool
	}, 0, len(yamlFiles))
	for _, filePath := range yamlFiles {
		koanfYamlFiles = append(koanfYamlFiles, struct {
			FileDir   string
			FilePath  string
			SkipError bool
		}{FilePath: filePath})
	}

	k := koanfpvd.NewKoanfProvider(&koanfpvd.KoanfConfig{
		YamlConfigFile: koanfYamlFiles,
		EnvPrefix:      "APP",
	})

	var root rootT
	if err := k.Unmarshall(&root); err != nil {
		return nil, fmt.Errorf("serverconfig: unmarshal: %w", err)
	}

	cfg := root.Server
	cfg.loadDefault()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *ServerConfigT) loadDefault() {
	if c.ClientAddr == "" {
		c.ClientAddr = ":8080"
	}
	if c.AdminAddr == "" {
		c.AdminAddr = ":8081"
	}
	if c.Repository == "" {
		c.Repository = RepositoryPostgres
	}
	if c.Auth.JWT.UserIDClaim == "" {
		c.Auth.JWT.UserIDClaim = "sub"
	}
	if c.Callbacks.SigningKeyFile == "" {
		c.Callbacks.SigningKeyFile = "webhook_ed25519.key"
	}
	if c.Callbacks.HandleMessage.Mode == "" {
		c.Callbacks.HandleMessage.Mode = HandleMsgModeSync
	}
	if c.Callbacks.HandleMessage.Timeout <= 0 {
		c.Callbacks.HandleMessage.Timeout = 5 * time.Second
	}
	if c.Callbacks.SyncTimeout <= 0 {
		c.Callbacks.SyncTimeout = 3 * time.Second
	}
	if c.Callbacks.AsyncRetryMax <= 0 {
		c.Callbacks.AsyncRetryMax = 6
	}
}

func (c *ServerConfigT) validate() error {
	if len(c.APIKeys) == 0 {
		return fmt.Errorf("serverconfig: SERVER.API_KEYS must not be empty")
	}
	switch c.Auth.Mode {
	case AuthModeWebhook:
	case AuthModeJWT:
		if c.Auth.JWT.JWKSURL == "" && c.Auth.JWT.PublicKeyPEMFile == "" {
			return fmt.Errorf("serverconfig: SERVER.AUTH.MODE=jwt requires JWKS_URL or PUBLIC_KEY_PEM_FILE")
		}
	default:
		return fmt.Errorf("serverconfig: SERVER.AUTH.MODE must be %q or %q, got %q", AuthModeJWT, AuthModeWebhook, c.Auth.Mode)
	}
	if c.Callbacks.BaseURL == "" {
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.BASE_URL is required")
	}
	switch c.Callbacks.HandleMessage.Mode {
	case HandleMsgModeSync, HandleMsgModeForward, HandleMsgModeDisabled:
	default:
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.HANDLE_MESSAGE.MODE must be sync|forward|disabled, got %q", c.Callbacks.HandleMessage.Mode)
	}
	switch c.Repository {
	case RepositoryPostgres, RepositoryDynamoDB:
	default:
		return fmt.Errorf("serverconfig: SERVER.REPOSITORY must be postgres|dynamodb, got %q", c.Repository)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/config/... -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add server/config/
git commit -m "feat(server): add SERVER config section loading and validation"
```

---

### Task 2: Webhook envelope + Ed25519 signer (`server/webhook`)

**Files:**
- Create: `server/webhook/envelope.go`
- Create: `server/webhook/signer.go`
- Test: `server/webhook/signer_test.go`

**Interfaces:**
- Produces (used by Tasks 3–8):
  - `webhook.SignatureHeader = "X-Pipewave-Signature"`
  - Event constants: `EventInspectToken="inspect_token"`, `EventHandleMessage="handle_message"`, `EventOnNewConnection="on_new_connection"`, `EventOnNewConnectionEstablished="on_new_connection_established"`, `EventOnCloseConnection="on_close_connection"`, `EventOnReadError="on_read_error"`, `EventOnWriteError="on_write_error"`, `EventMessageReceived="message_received"`
  - `type Meta struct { SentAt int64 \`json:"sent_at"\`; CallbackID string \`json:"id"\`; EventType string \`json:"event_type"\` }`
  - `type Body struct { Data json.RawMessage \`json:"data"\`; Meta Meta \`json:"meta"\` }`
  - `func NewCallbackID() string` — `"cb_" + nanoid`
  - `type PublicKeyVerifier struct { Alg string \`json:"alg"\`; PublicKeyInBase64 string \`json:"public_key_in_base64"\` }`
  - `func LoadOrGenerateSigner(keyFile string) (*Signer, error)`; methods `Sign(body []byte) string`, `Verify(body []byte, sigB64 string) bool`, `PublicKey() PublicKeyVerifier`

- [ ] **Step 1: Write the failing test**

```go
// server/webhook/signer_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/webhook/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

```go
// server/webhook/envelope.go
package webhook

import (
	"encoding/json"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// SignatureHeader carries the base64 Ed25519 signature over the raw body.
const SignatureHeader = "X-Pipewave-Signature"

// Event type discriminants carried in Meta.EventType. Class-1 (sync,
// response expected): inspect_token, handle_message, on_new_connection.
// Class-2 (async, fire-and-forget with retry): the rest.
const (
	EventInspectToken               = "inspect_token"
	EventHandleMessage              = "handle_message"
	EventOnNewConnection            = "on_new_connection"
	EventOnNewConnectionEstablished = "on_new_connection_established"
	EventOnCloseConnection          = "on_close_connection"
	EventOnReadError                = "on_read_error"
	EventOnWriteError               = "on_write_error"
	EventMessageReceived            = "message_received"
)

type Meta struct {
	SentAt     int64  `json:"sent_at"` // unix milliseconds
	CallbackID string `json:"id"`      // idempotency key; retries reuse it
	EventType  string `json:"event_type"`
}

type Body struct {
	Data json.RawMessage `json:"data"`
	Meta Meta            `json:"meta"`
}

func NewCallbackID() string {
	return "cb_" + gonanoid.Must()
}
```

```go
// server/webhook/signer.go
package webhook

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

type PublicKeyVerifier struct {
	Alg               string `json:"alg"`
	PublicKeyInBase64 string `json:"public_key_in_base64"`
}

type Signer struct {
	priv ed25519.PrivateKey
}

// LoadOrGenerateSigner loads the Ed25519 seed (base64, one line) from
// keyFile, or generates a new key pair and persists the seed with 0600 perms.
func LoadOrGenerateSigner(keyFile string) (*Signer, error) {
	b, err := os.ReadFile(keyFile)
	if err == nil {
		seed, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if decErr != nil {
			return nil, fmt.Errorf("webhook: signing key file %s is not base64: %w", keyFile, decErr)
		}
		if len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("webhook: signing key file %s: seed must be %d bytes, got %d", keyFile, ed25519.SeedSize, len(seed))
		}
		return &Signer{priv: ed25519.NewKeyFromSeed(seed)}, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("webhook: read signing key file %s: %w", keyFile, err)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("webhook: generate signing key: %w", err)
	}
	seedB64 := base64.StdEncoding.EncodeToString(priv.Seed())
	if err := os.WriteFile(keyFile, []byte(seedB64+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("webhook: persist signing key file %s: %w", keyFile, err)
	}
	return &Signer{priv: priv}, nil
}

func (s *Signer) Sign(body []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(s.priv, body))
}

func (s *Signer) Verify(body []byte, sigB64 string) bool {
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	return ed25519.Verify(s.priv.Public().(ed25519.PublicKey), body, sig)
}

func (s *Signer) PublicKey() PublicKeyVerifier {
	return PublicKeyVerifier{
		Alg:               "Ed25519",
		PublicKeyInBase64: base64.StdEncoding.EncodeToString(s.priv.Public().(ed25519.PublicKey)),
	}
}
```

Note: `gonanoid.Must()` is from the existing dependency `github.com/matoous/go-nanoid/v2`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/webhook/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/webhook/
git commit -m "feat(server): webhook envelope types and Ed25519 signer"
```

---

### Task 3: Signed HTTP sender (`server/webhook/sender.go`)

**Files:**
- Create: `server/webhook/sender.go`
- Test: `server/webhook/sender_test.go`

**Interfaces:**
- Consumes: `Signer`, `Body`, `Meta`, `SignatureHeader` (Task 2).
- Produces (used by Tasks 4, 5):
  - `func NewSender(url string, signer *Signer) *Sender`
  - `func (s *Sender) Post(ctx context.Context, eventType, callbackID string, data any, timeout time.Duration) (status int, respBody []byte, err error)`

- [ ] **Step 1: Write the failing test**

```go
// server/webhook/sender_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/webhook/ -run TestSender -v`
Expected: FAIL — `undefined: webhook.NewSender`.

- [ ] **Step 3: Write the implementation**

```go
// server/webhook/sender.go
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxRespBody = 1 << 20 // 1 MiB cap on callback responses

type Sender struct {
	httpClient *http.Client
	signer     *Signer
	url        string
}

func NewSender(url string, signer *Signer) *Sender {
	return &Sender{
		httpClient: &http.Client{},
		signer:     signer,
		url:        url,
	}
}

// Post marshals data into the signed envelope and POSTs it to the callback
// URL. The per-call timeout bounds the whole request. Retries (if any) are
// the caller's job — pass the same callbackID so receivers can dedupe.
func (s *Sender) Post(ctx context.Context, eventType, callbackID string, data any, timeout time.Duration) (int, []byte, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return 0, nil, fmt.Errorf("webhook: marshal data for %s: %w", eventType, err)
	}
	body, err := json.Marshal(Body{
		Data: raw,
		Meta: Meta{
			SentAt:     time.Now().UnixMilli(),
			CallbackID: callbackID,
			EventType:  eventType,
		},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("webhook: marshal envelope for %s: %w", eventType, err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("webhook: build request for %s: %w", eventType, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SignatureHeader, s.signer.Sign(body))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("webhook: post %s: %w", eventType, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBody))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("webhook: read response for %s: %w", eventType, err)
	}
	return resp.StatusCode, respBody, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/webhook/ -run TestSender -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/webhook/sender.go server/webhook/sender_test.go
git commit -m "feat(server): signed webhook HTTP sender"
```

---

### Task 4: Async dispatcher with retry (`server/webhook/async.go`)

**Files:**
- Create: `server/webhook/async.go`
- Test: `server/webhook/async_test.go`

**Interfaces:**
- Consumes: `Sender.Post`, `NewCallbackID` (Tasks 2–3).
- Produces (used by Tasks 6, 9):
  - `var DefaultBackoff = []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}`
  - `func NewAsyncDispatcher(sender *Sender, retryMax int, backoff []time.Duration) *AsyncDispatcher`
  - `func (d *AsyncDispatcher) Emit(eventType string, data any)` — non-blocking, drops when queue full
  - `func (d *AsyncDispatcher) Shutdown(ctx context.Context)` — best-effort drain

- [ ] **Step 1: Write the failing test**

```go
// server/webhook/async_test.go
package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

// collectServer fails the first `failures` requests with 500, then accepts.
type collectServer struct {
	mu       sync.Mutex
	failures int
	got      []webhook.Body
}

func (c *collectServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env webhook.Body
		_ = json.Unmarshal(body, &env)
		c.mu.Lock()
		defer c.mu.Unlock()
		c.got = append(c.got, env)
		if c.failures > 0 {
			c.failures--
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (c *collectServer) envelopes() []webhook.Body {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]webhook.Body(nil), c.got...)
}

func tinyBackoff() []time.Duration { return []time.Duration{time.Millisecond} }

func TestAsyncDispatcher_RetriesUntilSuccess_SameCallbackID(t *testing.T) {
	cs := &collectServer{failures: 2}
	srv := httptest.NewServer(cs.handler())
	defer srv.Close()

	d := webhook.NewAsyncDispatcher(webhook.NewSender(srv.URL, newTestSigner(t)), 6, tinyBackoff())
	d.Emit(webhook.EventOnCloseConnection, map[string]string{"user_id": "u1"})

	require.Eventually(t, func() bool { return len(cs.envelopes()) == 3 }, 2*time.Second, 5*time.Millisecond)
	d.Shutdown(context.Background())

	envs := cs.envelopes()
	for _, e := range envs {
		require.Equal(t, webhook.EventOnCloseConnection, e.Meta.EventType)
		require.Equal(t, envs[0].Meta.CallbackID, e.Meta.CallbackID, "retries must reuse the callback id")
	}
}

func TestAsyncDispatcher_DropsAfterMaxRetries(t *testing.T) {
	cs := &collectServer{failures: 1000}
	srv := httptest.NewServer(cs.handler())
	defer srv.Close()

	d := webhook.NewAsyncDispatcher(webhook.NewSender(srv.URL, newTestSigner(t)), 3, tinyBackoff())
	d.Emit(webhook.EventOnReadError, map[string]string{"user_id": "u1"})

	require.Eventually(t, func() bool { return len(cs.envelopes()) == 3 }, 2*time.Second, 5*time.Millisecond)
	time.Sleep(50 * time.Millisecond) // would-be 4th attempt window
	d.Shutdown(context.Background())
	require.Len(t, cs.envelopes(), 3, "must stop after retryMax attempts")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/webhook/ -run TestAsyncDispatcher -v`
Expected: FAIL — `undefined: webhook.NewAsyncDispatcher`.

- [ ] **Step 3: Write the implementation**

```go
// server/webhook/async.go
package webhook

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// DefaultBackoff is the retry schedule for async events; the last value
// repeats for attempts beyond its length.
var DefaultBackoff = []time.Duration{
	time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute,
}

const (
	asyncQueueSize   = 1024
	asyncPostTimeout = 10 * time.Second
)

type asyncJob struct {
	eventType  string
	callbackID string
	data       any
	attempt    int // delivery attempts already made
}

// AsyncDispatcher delivers Class-2 events at-least-once with in-memory
// retry. Events are dropped (with a warning log) when the queue is full,
// when retryMax is exhausted, or on shutdown/crash — accepted for v1.
type AsyncDispatcher struct {
	sender   *Sender
	backoff  []time.Duration
	retryMax int

	queue     chan asyncJob
	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func NewAsyncDispatcher(sender *Sender, retryMax int, backoff []time.Duration) *AsyncDispatcher {
	if len(backoff) == 0 {
		backoff = DefaultBackoff
	}
	d := &AsyncDispatcher{
		sender:   sender,
		backoff:  backoff,
		retryMax: retryMax,
		queue:    make(chan asyncJob, asyncQueueSize),
		closed:   make(chan struct{}),
	}
	d.wg.Add(1)
	go d.loop()
	return d
}

// Emit enqueues an event without blocking the caller (WS hot paths call
// this). A full queue drops the event.
func (d *AsyncDispatcher) Emit(eventType string, data any) {
	job := asyncJob{eventType: eventType, callbackID: NewCallbackID(), data: data}
	select {
	case d.queue <- job:
	default:
		slog.Warn("[webhook] async queue full, dropping event", "event_type", eventType)
	}
}

// Shutdown stops accepting retries and drains queued events best-effort
// until ctx expires.
func (d *AsyncDispatcher) Shutdown(ctx context.Context) {
	d.closeOnce.Do(func() { close(d.closed) })
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (d *AsyncDispatcher) loop() {
	defer d.wg.Done()
	for {
		select {
		case job := <-d.queue:
			d.deliver(job)
		case <-d.closed:
			for {
				select {
				case job := <-d.queue:
					d.deliver(job)
				default:
					return
				}
			}
		}
	}
}

func (d *AsyncDispatcher) deliver(job asyncJob) {
	status, _, err := d.sender.Post(context.Background(), job.eventType, job.callbackID, job.data, asyncPostTimeout)
	job.attempt++
	if err == nil && status >= 200 && status < 300 {
		return
	}

	if job.attempt >= d.retryMax {
		slog.Warn("[webhook] dropping event after max retries",
			"event_type", job.eventType, "callback_id", job.callbackID, "attempts", job.attempt, "last_status", status, "error", err)
		return
	}

	delay := d.backoff[min(job.attempt-1, len(d.backoff)-1)]
	time.AfterFunc(delay, func() {
		select {
		case <-d.closed:
		case d.queue <- job:
		default:
			slog.Warn("[webhook] async queue full, dropping retried event",
				"event_type", job.eventType, "callback_id", job.callbackID)
		}
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/webhook/ -run TestAsyncDispatcher -v -race`
Expected: PASS with no data races.

- [ ] **Step 5: Commit**

```bash
git add server/webhook/async.go server/webhook/async_test.go
git commit -m "feat(server): async webhook dispatcher with backoff retry"
```

---

### Task 5: Circuit breaker + sync caller (`server/webhook/sync.go`)

**Files:**
- Create: `server/webhook/sync.go`
- Test: `server/webhook/sync_test.go`

**Interfaces:**
- Consumes: `Sender.Post`, `NewCallbackID`.
- Produces (used by Tasks 6, 9):
  - `func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker` with `Allow() bool`, `Record(success bool)`
  - `var ErrCircuitOpen = errors.New(...)`
  - `type CallError struct { Status int; Body []byte }` implementing `error`
  - `func NewSyncCaller(sender *Sender, breaker *CircuitBreaker) *SyncCaller`
  - `func (c *SyncCaller) Call(ctx context.Context, eventType string, data any, timeout time.Duration, out any) error` — decodes 2xx JSON into `out` (nil ok); returns `*CallError` on non-2xx; only transport errors and 5xx count as breaker failures

- [ ] **Step 1: Write the failing test**

```go
// server/webhook/sync_test.go
package webhook_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)), webhook.NewCircuitBreaker(5, 10*time.Second))
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

	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)), webhook.NewCircuitBreaker(2, 10*time.Second))
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
	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)), breaker)

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
	c := webhook.NewSyncCaller(webhook.NewSender("http://127.0.0.1:1", newTestSigner(t)), webhook.NewCircuitBreaker(1, time.Minute))
	err := c.Call(context.Background(), webhook.EventInspectToken, nil, 50*time.Millisecond, nil)
	require.Error(t, err)
	require.False(t, errors.Is(err, webhook.ErrCircuitOpen))
	// breaker is now open
	require.ErrorIs(t, c.Call(context.Background(), webhook.EventInspectToken, nil, time.Second, nil), webhook.ErrCircuitOpen)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/webhook/ -run TestSyncCaller -v`
Expected: FAIL — `undefined: webhook.NewSyncCaller`.

- [ ] **Step 3: Write the implementation**

```go
// server/webhook/sync.go
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrCircuitOpen is returned without touching the backend while the
// breaker is open. Callers treat it like any other sync-call failure
// (fail closed / error frame).
var ErrCircuitOpen = errors.New("webhook: circuit breaker is open")

// CallError is a non-2xx answer from the callback receiver. 4xx bodies are
// application-level answers (e.g. rejected connection), not infrastructure
// failures.
type CallError struct {
	Status int
	Body   []byte
}

func (e *CallError) Error() string {
	return fmt.Sprintf("webhook: callback returned status %d: %s", e.Status, e.Body)
}

// CircuitBreaker opens after `threshold` consecutive infrastructure
// failures and lets traffic through again once `cooldown` has elapsed
// (all requests probe; the first success closes it).
type CircuitBreaker struct {
	mu        sync.Mutex
	threshold int
	cooldown  time.Duration
	failures  int
	openedAt  time.Time
	now       func() time.Time
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{threshold: threshold, cooldown: cooldown, now: time.Now}
}

func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures < b.threshold {
		return true
	}
	return b.now().Sub(b.openedAt) >= b.cooldown
}

func (b *CircuitBreaker) Record(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if success {
		b.failures = 0
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.openedAt = b.now()
	}
}

// SyncCaller performs Class-1 (request/response) callback invocations.
type SyncCaller struct {
	sender  *Sender
	breaker *CircuitBreaker
}

func NewSyncCaller(sender *Sender, breaker *CircuitBreaker) *SyncCaller {
	return &SyncCaller{sender: sender, breaker: breaker}
}

// Call posts the event and decodes a 2xx JSON response into out (out may
// be nil). Non-2xx returns *CallError. Only transport errors and 5xx are
// recorded as breaker failures.
func (c *SyncCaller) Call(ctx context.Context, eventType string, data any, timeout time.Duration, out any) error {
	if !c.breaker.Allow() {
		return ErrCircuitOpen
	}
	status, body, err := c.sender.Post(ctx, eventType, NewCallbackID(), data, timeout)
	if err != nil {
		c.breaker.Record(false)
		return err
	}
	if status < 200 || status >= 300 {
		c.breaker.Record(status < 500)
		return &CallError{Status: status, Body: body}
	}
	c.breaker.Record(true)
	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("webhook: decode %s response: %w", eventType, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/webhook/... -v -race`
Expected: PASS (whole package, including Tasks 2–4 tests).

- [ ] **Step 5: Commit**

```bash
git add server/webhook/sync.go server/webhook/sync_test.go
git commit -m "feat(server): sync webhook caller with circuit breaker"
```

---

### Task 6: JWT token inspector (`server/authn`)

**Files:**
- Create: `server/authn/jwt.go`
- Test: `server/authn/jwt_test.go`
- Modify: `go.mod` / `go.sum` (two new deps)

**Interfaces:**
- Produces (used by Task 9):
  - `type JWTConfig struct { JWKSURL, PublicKeyPEMFile, UserIDClaim string; MetadataClaims []string }`
  - `func NewJWTInspector(ctx context.Context, cfg JWTConfig) (*JWTInspector, error)`
  - `func (j *JWTInspector) InspectToken(ctx context.Context, token string, headers http.Header) (username string, isAnonymous bool, metadata map[string]string, err error)` — exactly the `types.Fns.InspectToken` signature

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/golang-jwt/jwt/v5 github.com/MicahParks/keyfunc/v3
```

- [ ] **Step 2: Write the failing test**

```go
// server/authn/jwt_test.go
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
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./server/authn/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 4: Write the implementation**

```go
// server/authn/jwt.go
package authn

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type JWTConfig struct {
	JWKSURL          string
	PublicKeyPEMFile string
	UserIDClaim      string
	MetadataClaims   []string
}

// JWTInspector implements the types.Fns.InspectToken contract by
// verifying a JWT locally — no callback round-trip per connection.
type JWTInspector struct {
	keyfunc jwt.Keyfunc
	cfg     JWTConfig
}

var allowedAlgs = []string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512", "EdDSA"}

// NewJWTInspector builds an inspector from a JWKS URL (refreshed in the
// background, bound to ctx) or a static PKIX public key PEM file.
func NewJWTInspector(ctx context.Context, cfg JWTConfig) (*JWTInspector, error) {
	if cfg.UserIDClaim == "" {
		cfg.UserIDClaim = "sub"
	}

	var kf jwt.Keyfunc
	switch {
	case cfg.JWKSURL != "":
		k, err := keyfunc.NewDefaultCtx(ctx, []string{cfg.JWKSURL})
		if err != nil {
			return nil, fmt.Errorf("authn: init JWKS from %s: %w", cfg.JWKSURL, err)
		}
		kf = k.Keyfunc
	case cfg.PublicKeyPEMFile != "":
		pub, err := loadPKIXPublicKey(cfg.PublicKeyPEMFile)
		if err != nil {
			return nil, err
		}
		kf = func(*jwt.Token) (any, error) { return pub, nil }
	default:
		return nil, errors.New("authn: JWTConfig requires JWKSURL or PublicKeyPEMFile")
	}

	return &JWTInspector{keyfunc: kf, cfg: cfg}, nil
}

func loadPKIXPublicKey(pemFile string) (any, error) {
	raw, err := os.ReadFile(pemFile)
	if err != nil {
		return nil, fmt.Errorf("authn: read public key file %s: %w", pemFile, err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("authn: %s is not PEM", pemFile)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("authn: parse PKIX public key from %s: %w", pemFile, err)
	}
	return pub, nil
}

// InspectToken satisfies the types.Fns.InspectToken signature.
func (j *JWTInspector) InspectToken(_ context.Context, token string, _ http.Header) (string, bool, map[string]string, error) {
	token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "Bearer "))

	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, j.keyfunc,
		jwt.WithValidMethods(allowedAlgs),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return "", false, nil, fmt.Errorf("authn: verify jwt: %w", err)
	}

	userID, _ := claims[j.cfg.UserIDClaim].(string)
	if userID == "" {
		return "", false, nil, fmt.Errorf("authn: claim %q missing or not a string", j.cfg.UserIDClaim)
	}

	var metadata map[string]string
	for _, name := range j.cfg.MetadataClaims {
		if v, ok := claims[name].(string); ok {
			if metadata == nil {
				metadata = map[string]string{}
			}
			metadata[name] = v
		}
	}
	return userID, false, metadata, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./server/authn/... -v && go mod tidy && go build ./...`
Expected: PASS; build clean.

- [ ] **Step 6: Commit**

```bash
git add server/authn/ go.mod go.sum
git commit -m "feat(server): built-in JWT inspect-token mode (JWKS or static PEM)"
```

---

### Task 7: Callback-backed Fns (`server/fns`)

**Files:**
- Create: `server/fns/fns.go`
- Test: `server/fns/fns_test.go`

**Interfaces:**
- Consumes: `webhook.SyncCaller.Call`, `webhook.AsyncDispatcher.Emit`, event constants (Tasks 4–5); `types.Fns`, `types.WebsocketAuth` (`export/types`); constants from `serverconfig` (Task 1).
- Produces (used by Task 9):
  - `type Config struct { HandleMessageMode string; HandleMessageTimeout, SyncTimeout time.Duration; InspectTokenOverride func(ctx context.Context, token string, headers http.Header) (string, bool, map[string]string, error) }` — `InspectTokenOverride` non-nil = JWT mode (Task 6's `InspectToken`); nil = webhook mode
  - `func New(syncCaller *webhook.SyncCaller, async *webhook.AsyncDispatcher, cfg Config) *types.Fns`
  - Wire DTO shapes (also the receiver contract, mirror them in `examples/rest-backend`):
    - auth: `{"user_id","instance_id","metadata"}`
    - `inspect_token` data: `{"token","headers"}` → response `{"user_id","is_anonymous","metadata"}`
    - `handle_message` / `message_received` data: `{"auth","input_type","data"(base64)}` → response `{"output_type","data"(base64)}`
    - `on_new_connection` / `on_new_connection_established` / `on_close_connection` data: `{"auth"}`
    - `on_read_error` / `on_write_error` data: `{"auth","error"}`

- [ ] **Step 1: Write the failing test**

```go
// server/fns/fns_test.go
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

	serverconfig "github.com/pipewave-dev/go-pkg/server/config"
	serverfns "github.com/pipewave-dev/go-pkg/server/fns"
	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/pipewave-dev/go-pkg/export/types"
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/fns/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

```go
// server/fns/fns.go
package serverfns

import (
	"context"
	"net/http"
	"time"

	serverconfig "github.com/pipewave-dev/go-pkg/server/config"
	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/pipewave-dev/go-pkg/export/types"
)

type Config struct {
	HandleMessageMode    string // serverconfig.HandleMsgMode*
	HandleMessageTimeout time.Duration
	SyncTimeout          time.Duration
	// InspectTokenOverride, when non-nil, replaces the inspect_token
	// webhook (JWT mode). Signature matches types.Fns.InspectToken.
	InspectTokenOverride func(ctx context.Context, token string, headers http.Header) (string, bool, map[string]string, error)
}

// Wire DTOs — this is the receiver-side contract; keep in lockstep with
// examples/rest-backend and the design doc.

type authDTO struct {
	UserID     string            `json:"user_id"`
	InstanceID string            `json:"instance_id"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func toAuthDTO(a types.WebsocketAuth) authDTO {
	return authDTO{UserID: a.UserID, InstanceID: a.InstanceID, Metadata: a.Metadata}
}

type inspectTokenReq struct {
	Token   string              `json:"token"`
	Headers map[string][]string `json:"headers,omitempty"`
}

type inspectTokenResp struct {
	UserID      string            `json:"user_id"`
	IsAnonymous bool              `json:"is_anonymous"`
	Metadata    map[string]string `json:"metadata"`
}

type handleMessageReq struct {
	Auth      authDTO `json:"auth"`
	InputType string  `json:"input_type"`
	Data      []byte  `json:"data"` // base64 in JSON
}

type handleMessageResp struct {
	OutputType string `json:"output_type"`
	Data       []byte `json:"data"`
}

type authEvent struct {
	Auth authDTO `json:"auth"`
}

type errorEvent struct {
	Auth  authDTO `json:"auth"`
	Error string  `json:"error"`
}

type webhookFns struct {
	sync  *webhook.SyncCaller
	async *webhook.AsyncDispatcher
	cfg   Config
}

// New builds the *types.Fns that bridges pipewave hooks to HTTP callbacks.
func New(syncCaller *webhook.SyncCaller, async *webhook.AsyncDispatcher, cfg Config) *types.Fns {
	w := &webhookFns{sync: syncCaller, async: async, cfg: cfg}
	return &types.Fns{
		InspectToken:      w.inspectToken,
		HandleMessage:     w,
		OnNewConnection:   w,
		OnCloseConnection: w,
		OnReadError:       w,
		OnWriteError:      w,
	}
}

func (w *webhookFns) inspectToken(ctx context.Context, token string, headers http.Header) (string, bool, map[string]string, error) {
	if w.cfg.InspectTokenOverride != nil {
		return w.cfg.InspectTokenOverride(ctx, token, headers)
	}
	var resp inspectTokenResp
	// Any failure (transport, 4xx, 5xx, open breaker) fails closed.
	if err := w.sync.Call(ctx, webhook.EventInspectToken, inspectTokenReq{Token: token, Headers: headers}, w.cfg.SyncTimeout, &resp); err != nil {
		return "", false, nil, err
	}
	return resp.UserID, resp.IsAnonymous, resp.Metadata, nil
}

func (w *webhookFns) HandleMessage(ctx context.Context, auth types.WebsocketAuth, inputType string, data []byte) (string, []byte, error) {
	req := handleMessageReq{Auth: toAuthDTO(auth), InputType: inputType, Data: data}
	switch w.cfg.HandleMessageMode {
	case serverconfig.HandleMsgModeForward:
		w.async.Emit(webhook.EventMessageReceived, req)
		return "", nil, nil
	case serverconfig.HandleMsgModeDisabled:
		return "", nil, nil
	default: // sync
		var resp handleMessageResp
		if err := w.sync.Call(ctx, webhook.EventHandleMessage, req, w.cfg.HandleMessageTimeout, &resp); err != nil {
			return "", nil, err // surfaces as an error frame to the client
		}
		return resp.OutputType, resp.Data, nil
	}
}

func (w *webhookFns) OnNewConnection(ctx context.Context, auth types.WebsocketAuth) error {
	// Fail closed: only a 2xx from the backend admits the connection.
	if err := w.sync.Call(ctx, webhook.EventOnNewConnection, authEvent{Auth: toAuthDTO(auth)}, w.cfg.SyncTimeout, nil); err != nil {
		return err
	}
	w.async.Emit(webhook.EventOnNewConnectionEstablished, authEvent{Auth: toAuthDTO(auth)})
	return nil
}

func (w *webhookFns) OnCloseConnection(_ context.Context, auth types.WebsocketAuth) {
	w.async.Emit(webhook.EventOnCloseConnection, authEvent{Auth: toAuthDTO(auth)})
}

func (w *webhookFns) OnReadError(_ context.Context, auth types.WebsocketAuth, err error) {
	w.async.Emit(webhook.EventOnReadError, errorEvent{Auth: toAuthDTO(auth), Error: err.Error()})
}

func (w *webhookFns) OnWriteError(_ context.Context, auth types.WebsocketAuth, err error) {
	w.async.Emit(webhook.EventOnWriteError, errorEvent{Auth: toAuthDTO(auth), Error: err.Error()})
}
```

Note: `types.Fns.OnNewConnection` etc. are interfaces (`types.OnNewConnectionT`, ...); `webhookFns` satisfies all of them, so it is assigned to each field.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/fns/... -v -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/fns/
git commit -m "feat(server): callback-backed types.Fns (sync + async webhook hooks)"
```

---

### Task 8: Admin REST API (`server/restapi`)

**Files:**
- Create: `server/restapi/respond.go`
- Create: `server/restapi/handlers.go`
- Create: `server/restapi/mux.go`
- Test: `server/restapi/restapi_test.go`

**Interfaces:**
- Consumes: `delivery.ModuleDelivery`, `delivery.ExportedServices`, `business.Monitoring` (`core/delivery`, `core/service/business`), `aerror` (`shared/aerror`), `webhook.PublicKeyVerifier` (Task 2).
- Produces (used by Task 9 and by Go embedders — this is the public `NewAdminMux` from the spec):
  - `type MuxConfig struct { APIKeys []string; PublicKey webhook.PublicKeyVerifier }`
  - `func NewAdminMux(pw delivery.ModuleDelivery, cfg MuxConfig) *http.ServeMux`
- Routes (spec's REST API mapping table, all JSON):
  - `POST /api/v1/messages/session` `{user_id, instance_id, msg_type, payload(base64), ack_timeout_ms?}` → `{"sent":true}` or `{"acked":bool}`
  - `POST /api/v1/messages/user` `{user_id, msg_type, payload, ack_timeout_ms?}` → same
  - `POST /api/v1/messages/users` `{user_ids[], msg_type, payload}` → `{"sent":true}`
  - `POST /api/v1/messages/broadcast` `{target:"all"|"authenticated"|"anonymous", msg_type, payload, instance_ids?[]}` → `{"sent":true}`
  - `DELETE /api/v1/sessions/{user_id}/{instance_id}`, `DELETE /api/v1/sessions/{user_id}` → `{"disconnected":true}`
  - `GET /api/v1/sessions/{user_id}` → `{"sessions":[{instance_id,holder_id,connection_type,status,connected_at}]}`
  - `GET /api/v1/presence/{user_id}` → `{"online":bool}`; `POST /api/v1/presence/batch` `{user_ids[]}` → `{"results":{id:bool}}`
  - `POST /api/v1/maintenance/cleanup` → `{"ok":true}`
  - `GET /api/v1/monitoring/connections` → `{"inside":{"anonymous_connections","user_connections","total_users"},"total":n}`
  - `GET /api/v1/monitoring/worker-pool` → `{"length","capacity","dropped"}`
  - `GET /api/v1/webhook/public-key` → `{"alg","public_key_in_base64"}`
  - `GET /healthz` (no auth) → 200 `{"healthy":true}` / 503 `{"healthy":false}`

- [ ] **Step 1: Write the failing test**

```go
// server/restapi/restapi_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/restapi/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

```go
// server/restapi/respond.go
package restapi

import (
	"encoding/json"
	"net/http"

	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errBody struct {
	Error errDetail `json:"error"`
}

type errDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeAError(w http.ResponseWriter, aErr aerror.AError) {
	code := aErr.ErrorCode()
	writeJSON(w, code.HttpCode(), errBody{Error: errDetail{Code: code.String(), Message: aErr.Error()}})
}

func writeBadRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, errBody{Error: errDetail{Code: aerror.ErrInvalidInput.String(), Message: msg}})
}

func writeUnauthorized(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, errBody{Error: errDetail{Code: aerror.LogicErrMissingAuthHeader.String(), Message: "missing or invalid API key"}})
}

// decodeBody strictly decodes the JSON request body into dst.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeBadRequest(w, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}
```

```go
// server/restapi/handlers.go
package restapi

import (
	"net/http"
	"time"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	business "github.com/pipewave-dev/go-pkg/core/service/business"
	"github.com/pipewave-dev/go-pkg/server/webhook"
)

type handlers struct {
	svc       delivery.ExportedServices
	mon       business.Monitoring
	publicKey webhook.PublicKeyVerifier
}

type sendResult struct {
	Sent  *bool `json:"sent,omitempty"`
	Acked *bool `json:"acked,omitempty"`
}

func sentResult() sendResult  { v := true; return sendResult{Sent: &v} }
func ackedResult(ok bool) sendResult { return sendResult{Acked: &ok} }

type sendToSessionReq struct {
	UserID       string `json:"user_id"`
	InstanceID   string `json:"instance_id"`
	MsgType      string `json:"msg_type"`
	Payload      []byte `json:"payload"`
	AckTimeoutMs int    `json:"ack_timeout_ms"`
}

func (h *handlers) sendToSession(w http.ResponseWriter, r *http.Request) {
	var req sendToSessionReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.UserID == "" || req.InstanceID == "" || req.MsgType == "" {
		writeBadRequest(w, "user_id, instance_id and msg_type are required")
		return
	}
	if req.AckTimeoutMs > 0 {
		acked, aErr := h.svc.SendToSessionWithAck(r.Context(), req.UserID, req.InstanceID, req.MsgType, req.Payload,
			time.Duration(req.AckTimeoutMs)*time.Millisecond)
		if aErr != nil {
			writeAError(w, aErr)
			return
		}
		writeJSON(w, http.StatusOK, ackedResult(acked))
		return
	}
	if aErr := h.svc.SendToSession(r.Context(), req.UserID, req.InstanceID, req.MsgType, req.Payload); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, sentResult())
}

type sendToUserReq struct {
	UserID       string `json:"user_id"`
	MsgType      string `json:"msg_type"`
	Payload      []byte `json:"payload"`
	AckTimeoutMs int    `json:"ack_timeout_ms"`
}

func (h *handlers) sendToUser(w http.ResponseWriter, r *http.Request) {
	var req sendToUserReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.UserID == "" || req.MsgType == "" {
		writeBadRequest(w, "user_id and msg_type are required")
		return
	}
	if req.AckTimeoutMs > 0 {
		acked, aErr := h.svc.SendToUserWithAck(r.Context(), req.UserID, req.MsgType, req.Payload,
			time.Duration(req.AckTimeoutMs)*time.Millisecond)
		if aErr != nil {
			writeAError(w, aErr)
			return
		}
		writeJSON(w, http.StatusOK, ackedResult(acked))
		return
	}
	if aErr := h.svc.SendToUser(r.Context(), req.UserID, req.MsgType, req.Payload); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, sentResult())
}

type sendToUsersReq struct {
	UserIDs []string `json:"user_ids"`
	MsgType string   `json:"msg_type"`
	Payload []byte   `json:"payload"`
}

func (h *handlers) sendToUsers(w http.ResponseWriter, r *http.Request) {
	var req sendToUsersReq
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.UserIDs) == 0 || req.MsgType == "" {
		writeBadRequest(w, "user_ids and msg_type are required")
		return
	}
	if aErr := h.svc.SendToUsers(r.Context(), req.UserIDs, req.MsgType, req.Payload); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, sentResult())
}

type broadcastReq struct {
	Target      string   `json:"target"` // all | authenticated | anonymous
	MsgType     string   `json:"msg_type"`
	Payload     []byte   `json:"payload"`
	InstanceIDs []string `json:"instance_ids"`
}

func (h *handlers) broadcast(w http.ResponseWriter, r *http.Request) {
	var req broadcastReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.MsgType == "" {
		writeBadRequest(w, "msg_type is required")
		return
	}
	switch req.Target {
	case "all":
		if e := h.svc.SendToAll(r.Context(), req.MsgType, req.Payload); e != nil {
			writeAError(w, e)
			return
		}
	case "authenticated":
		if e := h.svc.SendToAuthenticated(r.Context(), req.MsgType, req.Payload); e != nil {
			writeAError(w, e)
			return
		}
	case "anonymous":
		if e := h.svc.SendToAnonymous(r.Context(), req.MsgType, req.Payload, len(req.InstanceIDs) == 0, req.InstanceIDs); e != nil {
			writeAError(w, e)
			return
		}
	default:
		writeBadRequest(w, `target must be one of "all", "authenticated", "anonymous"`)
		return
	}
	writeJSON(w, http.StatusOK, sentResult())
}

func (h *handlers) disconnectSession(w http.ResponseWriter, r *http.Request) {
	if aErr := h.svc.DisconnectSession(r.Context(), r.PathValue("user_id"), r.PathValue("instance_id")); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"disconnected": true})
}

func (h *handlers) disconnectUser(w http.ResponseWriter, r *http.Request) {
	if aErr := h.svc.DisconnectUser(r.Context(), r.PathValue("user_id")); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"disconnected": true})
}

func (h *handlers) checkOnline(w http.ResponseWriter, r *http.Request) {
	online, aErr := h.svc.CheckOnline(r.Context(), r.PathValue("user_id"))
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"online": online})
}

type presenceBatchReq struct {
	UserIDs []string `json:"user_ids"`
}

func (h *handlers) checkOnlineBatch(w http.ResponseWriter, r *http.Request) {
	var req presenceBatchReq
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.UserIDs) == 0 {
		writeBadRequest(w, "user_ids is required")
		return
	}
	results, aErr := h.svc.CheckOnlineMultiple(r.Context(), req.UserIDs)
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

type sessionDTO struct {
	InstanceID     string    `json:"instance_id"`
	HolderID       string    `json:"holder_id"`
	ConnectionType string    `json:"connection_type"`
	Status         string    `json:"status"`
	ConnectedAt    time.Time `json:"connected_at"`
}

func (h *handlers) getUserSessions(w http.ResponseWriter, r *http.Request) {
	sessions, aErr := h.svc.GetUserSessions(r.Context(), r.PathValue("user_id"))
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	out := make([]sessionDTO, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionDTO{
			InstanceID:     s.InstanceID,
			HolderID:       s.HolderID,
			ConnectionType: s.ConnectionType.String(),
			Status:         s.Status.String(),
			ConnectedAt:    s.ConnectedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (h *handlers) cleanup(w http.ResponseWriter, r *http.Request) {
	if aErr := h.svc.CleanUp(r.Context()); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *handlers) monitoringConnections(w http.ResponseWriter, r *http.Request) {
	inside, aErr := h.mon.InsideActiveConnection(r.Context())
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	total, aErr := h.mon.TotalActiveConnection(r.Context())
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"inside": map[string]int{
			"anonymous_connections": inside.AnonymosConnection,
			"user_connections":      inside.UserConnection,
			"total_users":           inside.TotalUser,
		},
		"total": total,
	})
}

func (h *handlers) monitoringWorkerPool(w http.ResponseWriter, r *http.Request) {
	stats, aErr := h.mon.WorkerPoolStats(r.Context())
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"length":   stats.Length,
		"capacity": stats.Capacity,
		"dropped":  stats.Dropped,
	})
}

func (h *handlers) webhookPublicKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.publicKey)
}
```

```go
// server/restapi/mux.go
package restapi

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	"github.com/pipewave-dev/go-pkg/server/webhook"
)

type MuxConfig struct {
	APIKeys   []string
	PublicKey webhook.PublicKeyVerifier
}

// NewAdminMux builds the admin REST API from the public ModuleDelivery
// surface. The container mounts it on the admin listener; Go embedders can
// mount it into their own server (spec: "embedded admin API in scope").
func NewAdminMux(pw delivery.ModuleDelivery, cfg MuxConfig) *http.ServeMux {
	h := &handlers{svc: pw.Services(), mon: pw.Monitoring(), publicKey: cfg.PublicKey}

	api := http.NewServeMux()
	api.HandleFunc("POST /api/v1/messages/session", h.sendToSession)
	api.HandleFunc("POST /api/v1/messages/user", h.sendToUser)
	api.HandleFunc("POST /api/v1/messages/users", h.sendToUsers)
	api.HandleFunc("POST /api/v1/messages/broadcast", h.broadcast)
	api.HandleFunc("DELETE /api/v1/sessions/{user_id}/{instance_id}", h.disconnectSession)
	api.HandleFunc("DELETE /api/v1/sessions/{user_id}", h.disconnectUser)
	api.HandleFunc("GET /api/v1/sessions/{user_id}", h.getUserSessions)
	api.HandleFunc("GET /api/v1/presence/{user_id}", h.checkOnline)
	api.HandleFunc("POST /api/v1/presence/batch", h.checkOnlineBatch)
	api.HandleFunc("POST /api/v1/maintenance/cleanup", h.cleanup)
	api.HandleFunc("GET /api/v1/monitoring/connections", h.monitoringConnections)
	api.HandleFunc("GET /api/v1/monitoring/worker-pool", h.monitoringWorkerPool)
	api.HandleFunc("GET /api/v1/webhook/public-key", h.webhookPublicKey)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", requireAPIKey(cfg.APIKeys, api))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if pw.IsHealthy() {
			writeJSON(w, http.StatusOK, map[string]bool{"healthy": true})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]bool{"healthy": false})
	})
	return mux
}

func requireAPIKey(keys []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
		if got == "" || !matchAnyKey(got, keys) {
			writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func matchAnyKey(got string, keys []string) bool {
	ok := false
	for _, k := range keys {
		// constant-time per key; iterate all keys regardless of match
		if subtle.ConstantTimeCompare([]byte(got), []byte(k)) == 1 {
			ok = true
		}
	}
	return ok
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/restapi/... -v && go vet ./server/...`
Expected: PASS, vet clean.

- [ ] **Step 5: Commit**

```bash
git add server/restapi/
git commit -m "feat(server): admin REST API mux wrapping ExportedServices and Monitoring"
```

---

### Task 9: Container entrypoint (`cmd/pipewave-server`) + example config

**Files:**
- Create: `cmd/pipewave-server/main.go`
- Create: `server-config.example.yaml`

**Interfaces:**
- Consumes: everything above, plus `pipewave.New` / `pipewave.ConfigFromYAML` (`pipewave.go`), repo adapters `export/adapters/repo/postgresql` (`PostgresRepo`) and `export/adapters/repo/dynamodb` (`DynamoRepo`), `export/adapters/queue/valkey` (`QueueValkey`), `export/adapters/pubsub/valkey` (`PubsubValkey`). Note: all four adapter packages are named `adapters` — import with aliases.
- Produces: the `pipewave-server` binary.

- [ ] **Step 1: Write main.go**

```go
// cmd/pipewave-server/main.go
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	pipewave "github.com/pipewave-dev/go-pkg"
	dynamorepo "github.com/pipewave-dev/go-pkg/export/adapters/repo/dynamodb"
	pgrepo "github.com/pipewave-dev/go-pkg/export/adapters/repo/postgresql"
	pubsubvalkey "github.com/pipewave-dev/go-pkg/export/adapters/pubsub/valkey"
	queuevalkey "github.com/pipewave-dev/go-pkg/export/adapters/queue/valkey"
	"github.com/pipewave-dev/go-pkg/server/authn"
	serverconfig "github.com/pipewave-dev/go-pkg/server/config"
	serverfns "github.com/pipewave-dev/go-pkg/server/fns"
	"github.com/pipewave-dev/go-pkg/server/restapi"
	"github.com/pipewave-dev/go-pkg/server/webhook"
)

func main() {
	configFlag := flag.String("config", "config.yaml", "comma-separated list of YAML config files (later override earlier)")
	flag.Parse()
	files := strings.Split(*configFlag, ",")

	srvCfg, err := serverconfig.Load(files)
	if err != nil {
		fatal("load server config", err)
	}

	pw := pipewave.New(pipewave.PipewaveConfig{
		ConfigStore:       pipewave.ConfigFromYAML(files),
		RepositoryFactory: repoAdapter(srvCfg.Repository),
		QueueFactory:      queuevalkey.QueueValkey,
		PubsubFactory:     pubsubvalkey.PubsubValkey,
	})

	signer, err := webhook.LoadOrGenerateSigner(srvCfg.Callbacks.SigningKeyFile)
	if err != nil {
		fatal("init webhook signer", err)
	}
	sender := webhook.NewSender(srvCfg.Callbacks.BaseURL, signer)
	async := webhook.NewAsyncDispatcher(sender, srvCfg.Callbacks.AsyncRetryMax, webhook.DefaultBackoff)
	syncCaller := webhook.NewSyncCaller(sender, webhook.NewCircuitBreaker(5, 10*time.Second))

	fnsCfg := serverfns.Config{
		HandleMessageMode:    srvCfg.Callbacks.HandleMessage.Mode,
		HandleMessageTimeout: srvCfg.Callbacks.HandleMessage.Timeout,
		SyncTimeout:          srvCfg.Callbacks.SyncTimeout,
	}
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	if srvCfg.Auth.Mode == serverconfig.AuthModeJWT {
		inspector, err := authn.NewJWTInspector(rootCtx, authn.JWTConfig{
			JWKSURL:          srvCfg.Auth.JWT.JWKSURL,
			PublicKeyPEMFile: srvCfg.Auth.JWT.PublicKeyPEMFile,
			UserIDClaim:      srvCfg.Auth.JWT.UserIDClaim,
			MetadataClaims:   srvCfg.Auth.JWT.MetadataClaims,
		})
		if err != nil {
			fatal("init jwt inspector", err)
		}
		fnsCfg.InspectTokenOverride = inspector.InspectToken
	}

	pw.SetFns(serverfns.New(syncCaller, async, fnsCfg))

	if err := pw.RunMigration(); err != nil {
		fatal("run migration", err)
	}

	clientSrv := &http.Server{Addr: srvCfg.ClientAddr, Handler: pw.Mux()}
	adminSrv := &http.Server{Addr: srvCfg.AdminAddr, Handler: restapi.NewAdminMux(pw, restapi.MuxConfig{
		APIKeys:   srvCfg.APIKeys,
		PublicKey: signer.PublicKey(),
	})}

	go serve("client", clientSrv)
	go serve("admin", adminSrv)

	<-rootCtx.Done()
	slog.Info("[pipewave-server] shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = adminSrv.Shutdown(shutdownCtx)
	_ = clientSrv.Shutdown(shutdownCtx)
	pw.Shutdown()
	async.Shutdown(shutdownCtx)
	slog.Info("[pipewave-server] bye")
}

func repoAdapter(name string) adapters.RepositoryAdapter {
	if name == serverconfig.RepositoryDynamoDB {
		return dynamorepo.DynamoRepo
	}
	return pgrepo.PostgresRepo
}

func serve(name string, srv *http.Server) {
	slog.Info("[pipewave-server] listening", "listener", name, "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal(name+" listener", err)
	}
}

func fatal(what string, err error) {
	slog.Error("[pipewave-server] fatal: "+what, "error", err)
	os.Exit(1)
}
```

Add `"github.com/pipewave-dev/go-pkg/export/adapters"` to the import block (it provides the `adapters.RepositoryAdapter` type returned by `repoAdapter`).

- [ ] **Step 2: Write the example config**

```yaml
# server-config.example.yaml — config for the pipewave-server container.
# Core sections (VALKEY, POSTGRES, WORKER_POOL, ...) are the same as
# example-config.yaml; SERVER is the container-only section.

AUTO_MIGRATION: true

WORKER_POOL:
  BUFFER: 100
  UPPER_THRESHOLD: 80
  LOWER_THRESHOLD: 20

VALKEY:
  PRIMARY_ADDRESS: "localhost:29100"
  REPLICA_ADDRESS: "localhost:29100"
  PASSWORD: "veryStrongP@ssw0rd"
  DATABASE_IDX: 0

POSTGRES:
  CREATE_TABLES: true
  HOST: "localhost"
  PORT: 29102
  DB_NAME: "postgres"
  USER: "postgres"
  PASSWORD: "postgres"
  SSL_MODE: "disable"
  MAX_CONNS: 15
  MIN_CONNS: 1

SERVER:
  CLIENT_ADDR: ":8080"
  ADMIN_ADDR: ":8081"
  # Scalar fields can be overridden via env, e.g. APP_SERVER__ADMIN_ADDR
  API_KEYS: ["change-me"]
  REPOSITORY: "postgres" # postgres | dynamodb
  AUTH:
    MODE: "webhook" # webhook | jwt
    JWT:
      JWKS_URL: ""
      PUBLIC_KEY_PEM_FILE: ""
      USER_ID_CLAIM: "sub"
      METADATA_CLAIMS: []
  CALLBACKS:
    BASE_URL: "http://localhost:9000/pipewave/callback"
    SIGNING_KEY_FILE: "webhook_ed25519.key"
    HANDLE_MESSAGE:
      MODE: "sync" # sync | forward | disabled
      TIMEOUT: "5s"
    SYNC_TIMEOUT: "3s"
    ASYNC_RETRY_MAX: 6
```

- [ ] **Step 3: Verify it compiles and vets**

Run: `go build ./... && go vet ./cmd/... ./server/...`
Expected: clean. (No unit test for `main`; it is pure wiring, covered by the smoke test in Task 10.)

- [ ] **Step 4: Commit**

```bash
git add cmd/pipewave-server/ server-config.example.yaml
git commit -m "feat(server): pipewave-server container entrypoint"
```

---

### Task 10: Example receiver, Dockerfile, smoke test, docs

**Files:**
- Create: `examples/rest-backend/main.go`
- Create: `Dockerfile`
- Modify: `docker-compose.yml` (add `pipewave-server` service)
- Modify: `README.md` (add a "Run as a container" section pointing at the spec and example)

**Interfaces:**
- Consumes: the wire DTOs from Task 7 (receiver side) and `webhook.SignatureHeader` / envelope shape (verification uses only stdlib — this example is what non-Go teams port).

- [ ] **Step 1: Write the example receiver**

```go
// examples/rest-backend/main.go
//
// Minimal callback receiver for pipewave-server. Verifies the Ed25519
// signature, answers the three sync events, and logs async events.
// Port this file to any language — it uses only the HTTP contract.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type meta struct {
	SentAt    int64  `json:"sent_at"`
	ID        string `json:"id"`
	EventType string `json:"event_type"`
}

type envelope struct {
	Data json.RawMessage `json:"data"`
	Meta meta            `json:"meta"`
}

func main() {
	addr := flag.String("addr", ":9000", "listen address")
	pubKeyB64 := flag.String("public-key", "", "pipewave webhook public key (base64); fetch from GET /api/v1/webhook/public-key")
	flag.Parse()

	var pubKey ed25519.PublicKey
	if *pubKeyB64 != "" {
		raw, err := base64.StdEncoding.DecodeString(*pubKeyB64)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			log.Fatalf("invalid public key: %v", err)
		}
		pubKey = raw
	}

	http.HandleFunc("POST /pipewave/callback", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

		// 1. Verify signature (skipped when no key is configured — dev only).
		if pubKey != nil {
			sig, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Pipewave-Signature"))
			if err != nil || !ed25519.Verify(pubKey, body, sig) {
				http.Error(w, "bad signature", http.StatusUnauthorized)
				return
			}
		}

		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			http.Error(w, "bad envelope", http.StatusBadRequest)
			return
		}

		// 2. Reject stale deliveries (replay protection).
		if time.Since(time.UnixMilli(env.Meta.SentAt)) > 5*time.Minute {
			http.Error(w, "stale", http.StatusUnauthorized)
			return
		}

		log.Printf("event=%s id=%s data=%s", env.Meta.EventType, env.Meta.ID, env.Data)

		// 3. Switch on event type; sync events return a JSON body.
		switch env.Meta.EventType {
		case "inspect_token":
			var in struct {
				Token string `json:"token"`
			}
			_ = json.Unmarshal(env.Data, &in)
			// Demo auth: the token IS the user id.
			writeJSON(w, map[string]any{"user_id": in.Token, "is_anonymous": in.Token == "", "metadata": nil})

		case "handle_message":
			var in struct {
				InputType string `json:"input_type"`
				Data      []byte `json:"data"`
			}
			_ = json.Unmarshal(env.Data, &in)
			// Demo handler: echo everything back.
			writeJSON(w, map[string]any{"output_type": in.InputType + "_RESPONSE", "data": in.Data})

		case "on_new_connection":
			w.WriteHeader(http.StatusOK) // 2xx admits the connection

		default: // async events: acknowledge receipt
			w.WriteHeader(http.StatusOK)
		}
	})

	fmt.Println("rest-backend listening on", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 2: Write the Dockerfile**

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pipewave-server ./cmd/pipewave-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/pipewave-server /pipewave-server
# Mount your config at /etc/pipewave/config.yaml (see server-config.example.yaml)
ENTRYPOINT ["/pipewave-server", "-config", "/etc/pipewave/config.yaml"]
```

- [ ] **Step 3: Add the compose service**

Read the existing `docker-compose.yml` first and append a service that reuses its Valkey/Postgres services (adjust service names to whatever the file actually uses — do not invent new ones):

```yaml
  pipewave-server:
    build: .
    ports:
      - "8080:8080"
      - "8081:8081"
    volumes:
      - ./server-config.example.yaml:/etc/pipewave/config.yaml:ro
    environment:
      APP_SERVER__CALLBACKS__BASE_URL: "http://host.docker.internal:9000/pipewave/callback"
      APP_VALKEY__PRIMARY_ADDRESS: "valkey:6379"   # match compose service name/port
      APP_VALKEY__REPLICA_ADDRESS: "valkey:6379"
      APP_POSTGRES__HOST: "postgres"               # match compose service name
      APP_POSTGRES__PORT: "5432"
```

- [ ] **Step 4: End-to-end smoke test (manual, verify before claiming done)**

```bash
# terminal 1: infra
docker compose up -d   # valkey + postgres

# terminal 2: stub backend
go run ./examples/rest-backend -addr :9000

# terminal 3: the server (config points BASE_URL at localhost:9000)
go run ./cmd/pipewave-server -config server-config.example.yaml

# terminal 4: drive it
# 4a. health
curl -s localhost:8081/healthz                                # {"healthy":true}
# 4b. admin auth
curl -s localhost:8081/api/v1/webhook/public-key              # 401
curl -s -H "Authorization: Bearer change-me" localhost:8081/api/v1/webhook/public-key
# 4c. websocket roundtrip (token "alice" → inspect_token callback → user alice)
#     issue-tmp-token then connect; easiest with the playground web client or websocat:
curl -s -X POST -H "Authorization: Bearer alice" localhost:8080/issue-tmp-token   # → {tk}
websocat "ws://localhost:8080/gw?tk=<tk>"
#     send a frame; expect the echo response produced by the stub backend,
#     and handle_message / on_new_connection lines in terminal 2's log.
# 4d. REST send while the socket is open
curl -s -X POST -H "Authorization: Bearer change-me" -H "Content-Type: application/json" \
  -d '{"user_id":"alice","msg_type":"NEWS","payload":"aGVsbG8="}' \
  localhost:8081/api/v1/messages/user                          # {"sent":true}
#     the frame appears on the websocket.
# 4e. presence
curl -s -H "Authorization: Bearer change-me" localhost:8081/api/v1/presence/alice  # {"online":true}
# 4f. close the socket → on_close_connection appears in terminal 2's log.
```

Note: the exact `issue-tmp-token` request/response shape may differ — check `core/service/websocket/mediator/delivery/` (`IssueTmpToken`) and adapt the curl accordingly. Record what you actually ran and saw.

- [ ] **Step 5: Docs**

Append to `README.md` a short "Run as a container" section: what `pipewave-server` is, link to `docs/feats/2026-07-17-rest-api-container-design.md`, the config example, the callback contract summary (envelope + signature header + event types), and the smoke-test commands above.

- [ ] **Step 6: Full verification and commit**

```bash
go build ./... && go vet ./... && go test ./server/... -race
docker build -t pipewave-server:dev .
git add examples/ Dockerfile docker-compose.yml README.md
git commit -m "feat(server): example callback receiver, Dockerfile, compose service, docs"
```

---

## Final acceptance checklist

- [ ] `go build ./... && go vet ./... && go test ./server/... -race` all green.
- [ ] Smoke test (Task 10 Step 4) performed end-to-end with real Valkey + Postgres; WS echo via callback and REST send both observed.
- [ ] Spec cross-check (`docs/feats/2026-07-17-rest-api-container-design.md`): every route in the REST mapping table exists; both `sync` and `forward` handle_message modes work; `inspect_token`/`on_new_connection` fail closed; async events retry and reuse `meta.id`; public key endpoint serves the signer's key; JWT mode validated by unit tests.
- [ ] No files under `core/`, `provider/`, `export/`, `shared/` were modified.
