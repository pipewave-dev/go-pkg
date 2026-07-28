# Metrics Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose Prometheus metrics for connection lifecycle, inbound client messages, and webhook callbacks on a dedicated `:9090` listener.

**Architecture:** `pkg/metrics` creates OTEL instruments from `otel.GetMeterProvider()` and never sets it — so Go embedders keep ownership of their registry and get a no-op when they set no provider. The container's new `provider/metrics-provider` is the only place that builds a Prometheus exporter + MeterProvider + HTTP listener. `server/webhook` never imports `pkg/metrics`; it takes an optional `CallObserver` interface, wired only in `cmd/pipewave-server/main.go`.

**Tech Stack:** Go 1.25.5, `go.opentelemetry.io/otel` (metric API), `go.opentelemetry.io/otel/sdk/metric`, `go.opentelemetry.io/otel/exporters/prometheus`, `github.com/prometheus/client_golang` (promhttp), `github.com/samber/do/v2` (DI), `github.com/stretchr/testify/require`.

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-07-28-metrics-observability-design.md`. Read it before starting.
- **Metrics must never break the main path.** No `panic` on instrument creation error — log warn and continue with a no-op. Listener bind failure logs an error; the server keeps running.
- **`pkg/metrics` MUST NOT call `otel.SetMeterProvider()`** and MUST NOT import Prometheus or `net/http`. It only reads `otel.GetMeterProvider()`.
- **`server/webhook` MUST NOT import `pkg/metrics`.** Communication is via the `CallObserver` interface defined inside `webhook`.
- **Never use high-cardinality labels.** `user_id`, `session_id`, `instance_id` are forbidden as labels. `container_id` appears ONLY on `pipewave_build_info`.
- **All new config is opt-in.** `METRICS.ENABLED` defaults to `false`; default behaviour is identical to today.
- **Existing tests must keep passing.** `server/webhook` tests must not need modification (observer is nil-able).
- **Metric naming:** `pipewave_` prefix, `_total` suffix for counters, `_seconds` for durations, base units only.
- Test command: `go test ./<pkg>/... -run <TestName> -v`. Lint: `golangci-lint run ./<pkg>/...`.
- Go module path is `github.com/pipewave-dev/go-pkg` (NOT `pipewave-gopkg`).

---

## File Structure

**Create:**
| File | Responsibility |
|---|---|
| `pkg/metrics/sanitize.go` | `sanitizeMsgType`, `classifyCallbackError` — pure functions, no OTEL |
| `pkg/metrics/sanitize_test.go` | Table-driven tests for both |
| `pkg/metrics/metrics.go` | **rewrite**: Tier 1 instruments from global provider |
| `pkg/metrics/metrics_test.go` | ManualReader assertions + no-op safety |
| `pkg/metrics/observable.go` | ObservableGauge callbacks reading `business.Monitoring` |
| `pkg/metrics/observable_test.go` | Stub Monitoring, cached-value-on-error |
| `pkg/metrics/callback.go` | Tier 2 instruments + `CallbackMetrics` implementing `webhook.CallObserver` |
| `pkg/metrics/callback_test.go` | ManualReader assertions for callback metrics |
| `provider/metrics-provider/metrics.go` | Prometheus exporter + MeterProvider + `:9090` listener + cleanup task |
| `provider/metrics-provider/metrics_test.go` | Real listener on port 0, assert `/metrics` body |

**Modify:**
| File | Change |
|---|---|
| `export/types/config_child.go` | Add `MetricsT` + `validate()` + `loadDefault()` |
| `export/types/config.go` | Add `Metrics *MetricsT` field; call validate/loadDefault |
| `server/webhook/sender.go` | Add `CallObserver` interface + `WithObserver` + observe in `Post` |
| `server/webhook/sync.go` | Count retries via observer |
| `server/webhook/async.go` | Count retries + dropped via observer |
| `server/webhook/sender_observer_test.go` | New test file for observer hook |
| `core/service/websocket/mediator/delivery/0.new.go` | Track conn open time; record duration on close |
| `core/service/websocket/mediator/delivery/2.gobwas_endpoint.go` | Record accepted/rejected with `reason` |
| `core/service/websocket/client-msg-handler/0_main_handler.go` | Record message count/duration/outcome |
| `core/delivery/module/0.0.new.go` | Remove `metrics.New()` (moves to DI) |
| `do_packages.go` | Register `metricsprovider.NewDI` |
| `cmd/pipewave-server/main.go` | Start metrics listener; wire callback observer |
| `server-config.example.yaml` | Document `METRICS` block |
| `README.md` | Document `/metrics` endpoint |

**Task ordering rationale:** Task 1 (pure functions) and Task 2 (config) have no dependencies. Task 3 (instruments) depends on Task 1. Task 4 (provider) depends on Task 2. Tasks 5-7 (call sites) depend on Task 3. Task 8 wires everything.

---

## Task 1: Sanitize & classify pure functions

**Files:**
- Create: `pkg/metrics/sanitize.go`
- Test: `pkg/metrics/sanitize_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func SanitizeMsgType(raw string, allowlist map[string]struct{}) string`
  - `func BuildAllowlist(entries []string) map[string]struct{}`
  - `func ClassifyCallbackError(err error, statusCode int) string`
  - Constants: `MsgTypeOther = "other"`, `MsgTypeHeartbeat = "heartbeat"`, `MsgTypeAck = "ack"`

**Background you need:** `core/service/websocket/0.message_type.go` defines
`MessageType` as a `string` whose system values are a **single non-printable
byte**: `MessageTypeHeartbeat = MessageType([]byte{202})` and
`MessageTypeAck = MessageType([]byte{203})`. App-level values are arbitrary
client-supplied strings from msgpack. Raw bytes are unreadable in Grafana and
unbounded cardinality, so every value must pass through `SanitizeMsgType`.

- [ ] **Step 1: Write the failing test**

Create `pkg/metrics/sanitize_test.go`:

```go
package metrics_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"

	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/stretchr/testify/require"
)

func TestSanitizeMsgType(t *testing.T) {
	allow := metrics.BuildAllowlist([]string{"CHAT", "NEWS"})

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"heartbeat system byte", string([]byte{202}), metrics.MsgTypeHeartbeat},
		{"ack system byte", string([]byte{203}), metrics.MsgTypeAck},
		{"allowlisted", "CHAT", "CHAT"},
		{"allowlisted second", "NEWS", "NEWS"},
		{"not allowlisted", "SECRET_TYPE", metrics.MsgTypeOther},
		{"empty", "", metrics.MsgTypeOther},
		{"non printable not system", string([]byte{7}), metrics.MsgTypeOther},
		{"too long even if allowlisted-looking", strings.Repeat("A", 33), metrics.MsgTypeOther},
		{"exactly 32 but not allowlisted", strings.Repeat("A", 32), metrics.MsgTypeOther},
		{"embedded newline", "CH\nAT", metrics.MsgTypeOther},
		{"unicode", "chào", metrics.MsgTypeOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, metrics.SanitizeMsgType(tt.raw, allow))
		})
	}
}

func TestSanitizeMsgType_EmptyAllowlistCollapsesAppTypes(t *testing.T) {
	allow := metrics.BuildAllowlist(nil)
	require.Equal(t, metrics.MsgTypeOther, metrics.SanitizeMsgType("CHAT", allow))
	// system types still resolve without an allowlist
	require.Equal(t, metrics.MsgTypeHeartbeat, metrics.SanitizeMsgType(string([]byte{202}), allow))
}

func TestClassifyCallbackError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		want   string
	}{
		{"nil error 200", nil, 200, ""},
		{"deadline exceeded", context.DeadlineExceeded, 0, "timeout"},
		{"wrapped deadline", fmt.Errorf("post failed: %w", context.DeadlineExceeded), 0, "timeout"},
		{"conn refused", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), 0, "conn_refused"},
		{"dns error", &net.DNSError{Err: "no such host"}, 0, "dns"},
		{"wrapped dns", fmt.Errorf("lookup: %w", &net.DNSError{Err: "nope"}), 0, "dns"},
		{"status 500", nil, 500, "status_5xx"},
		{"status 503", nil, 503, "status_5xx"},
		{"status 404", nil, 404, "status_4xx"},
		{"status 400", nil, 400, "status_4xx"},
		{"unknown error", errors.New("boom"), 0, "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, metrics.ClassifyCallbackError(tt.err, tt.status))
		})
	}
}

func TestClassifyCallbackError_ErrorWinsOverStatus(t *testing.T) {
	// transport error with a zero status must not be read as status_4xx
	require.Equal(t, "timeout", metrics.ClassifyCallbackError(context.DeadlineExceeded, 0))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/metrics/... -run 'TestSanitizeMsgType|TestClassifyCallbackError' -v`
Expected: FAIL — build error, `undefined: metrics.SanitizeMsgType`.

- [ ] **Step 3: Write the implementation**

Create `pkg/metrics/sanitize.go`:

```go
// Package metrics creates OTEL metric instruments for pipewave.
//
// It reads the global MeterProvider via otel.GetMeterProvider() and never
// sets it: the process that embeds pipewave owns that decision. When no
// provider is configured the OTEL API returns no-op instruments, so every
// Record* call is a cheap no-op and pipewave adds no metrics overhead.
package metrics

import (
	"context"
	"errors"
	"net"
	"net/http"
	"syscall"
)

// Label values for the msg_type dimension.
const (
	MsgTypeOther     = "other"
	MsgTypeHeartbeat = "heartbeat"
	MsgTypeAck       = "ack"
)

// System message types are single non-printable bytes on the wire
// (core/service/websocket/0.message_type.go). Map them to readable labels.
const (
	sysByteHeartbeat = 202
	sysByteAck       = 203
)

// maxMsgTypeLen bounds label length so a hostile client cannot inflate
// Prometheus memory with long label values.
const maxMsgTypeLen = 32

// BuildAllowlist converts configured msg_type entries into a lookup set.
// A nil or empty slice yields an empty set, which collapses every
// app-level msg_type to MsgTypeOther.
func BuildAllowlist(entries []string) map[string]struct{} {
	set := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		set[e] = struct{}{}
	}
	return set
}

// SanitizeMsgType maps a wire msg_type to a bounded, readable label value.
//
// msg_type is client-controlled, so it is never used as a label verbatim:
// unknown values collapse to MsgTypeOther. This keeps cardinality bounded by
// len(allowlist)+3 regardless of what clients send.
func SanitizeMsgType(raw string, allowlist map[string]struct{}) string {
	if len(raw) == 1 {
		switch raw[0] {
		case sysByteHeartbeat:
			return MsgTypeHeartbeat
		case sysByteAck:
			return MsgTypeAck
		}
	}
	if raw == "" || len(raw) > maxMsgTypeLen {
		return MsgTypeOther
	}
	if _, ok := allowlist[raw]; !ok {
		return MsgTypeOther
	}
	if !isPrintableASCII(raw) {
		return MsgTypeOther
	}
	return raw
}

// isPrintableASCII reports whether s consists solely of printable ASCII
// (0x20-0x7E). Anything else would render as garbage in a dashboard.
func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7E {
			return false
		}
	}
	return true
}

// Label values for the callback reason dimension.
const (
	ReasonTimeout     = "timeout"
	ReasonConnRefused = "conn_refused"
	ReasonDNS         = "dns"
	ReasonStatus4xx   = "status_4xx"
	ReasonStatus5xx   = "status_5xx"
	ReasonBadBody     = "bad_body"
	ReasonBreakerOpen = "breaker_open"
	ReasonOther       = "other"
)

// ClassifyCallbackError reduces a callback failure to a bounded reason label.
// Returns "" when the call succeeded (nil err and a non-error status).
//
// Transport errors are classified before status codes: a timeout carries
// statusCode 0, which must not be misread as a 4xx.
func ClassifyCallbackError(err error, statusCode int) string {
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return ReasonTimeout
		case errors.Is(err, syscall.ECONNREFUSED):
			return ReasonConnRefused
		}
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			return ReasonDNS
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return ReasonTimeout
		}
		return ReasonOther
	}
	switch {
	case statusCode >= http.StatusInternalServerError:
		return ReasonStatus5xx
	case statusCode >= http.StatusBadRequest:
		return ReasonStatus4xx
	}
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/metrics/... -run 'TestSanitizeMsgType|TestClassifyCallbackError' -v`
Expected: PASS, all subtests.

- [ ] **Step 5: Lint**

Run: `golangci-lint run ./pkg/metrics/...`
Expected: no findings.

- [ ] **Step 6: Commit**

```bash
git add pkg/metrics/sanitize.go pkg/metrics/sanitize_test.go
git commit -m "feat(metrics): msg_type sanitizer and callback error classifier"
```

---

## Task 2: METRICS config block

**Files:**
- Modify: `export/types/config_child.go` (append after `OtelT.loadDefault`, around line 317)
- Modify: `export/types/config.go:20` (field), `:34` (validate), `:46` (loadDefault)
- Test: `export/types/metrics_config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `types.MetricsT` with exported fields `Enabled bool`, `Port int`, `Path string`, `MsgTypeAllowlist []string`. Reached at runtime via `cfg.Env().Metrics`.

**Background:** `export/types/config.go` holds `EnvType`. Each child struct has
private `validate()` / `loadDefault()` called from `EnvType.Validate()` /
`EnvType.LoadDefault()`. Follow `OtelT` exactly: validation panics (fail fast at
boot). Note `Metrics` is a pointer field, so `loadDefault` must handle nil.

- [ ] **Step 1: Write the failing test**

Create `export/types/metrics_config_test.go`:

```go
package types_test

import (
	"testing"

	"github.com/pipewave-dev/go-pkg/export/types"
	"github.com/stretchr/testify/require"
)

func TestMetricsT_Defaults(t *testing.T) {
	m := &types.MetricsT{}
	m.LoadDefaultForTest()
	require.Equal(t, 9090, m.Port)
	require.Equal(t, "/metrics", m.Path)
	require.False(t, m.Enabled)
}

func TestMetricsT_ValidateDisabledSkipsChecks(t *testing.T) {
	m := &types.MetricsT{Enabled: false, Port: -1, Path: "nope"}
	require.NotPanics(t, m.ValidateForTest)
}

func TestMetricsT_ValidateEnabled(t *testing.T) {
	tests := []struct {
		name      string
		m         types.MetricsT
		wantPanic bool
	}{
		{"valid", types.MetricsT{Enabled: true, Port: 9090, Path: "/metrics"}, false},
		{"port zero", types.MetricsT{Enabled: true, Port: 0, Path: "/metrics"}, true},
		{"port too high", types.MetricsT{Enabled: true, Port: 70000, Path: "/metrics"}, true},
		{"path missing slash", types.MetricsT{Enabled: true, Port: 9090, Path: "metrics"}, true},
		{"allowlist too long", types.MetricsT{
			Enabled: true, Port: 9090, Path: "/metrics",
			MsgTypeAllowlist: []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, // 33
		}, true},
		{"allowlist non printable", types.MetricsT{
			Enabled: true, Port: 9090, Path: "/metrics",
			MsgTypeAllowlist: []string{string([]byte{7})},
		}, true},
		{"allowlist ok", types.MetricsT{
			Enabled: true, Port: 9090, Path: "/metrics",
			MsgTypeAllowlist: []string{"CHAT", "NEWS"},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.m
			if tt.wantPanic {
				require.Panics(t, m.ValidateForTest)
				return
			}
			require.NotPanics(t, m.ValidateForTest)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./export/types/... -run TestMetricsT -v`
Expected: FAIL — `undefined: types.MetricsT`.

- [ ] **Step 3: Add MetricsT**

Append to `export/types/config_child.go` (after `OtelT.loadDefault`, ~line 317):

```go
// MetricsT contains Prometheus metrics configuration.
//
// Metrics are served on their own listener, separate from the client and admin
// listeners, so Prometheus can scrape without being granted admin API access.
type MetricsT struct {
	Enabled bool   `koanf:"ENABLED"`
	Port    int    `koanf:"PORT"`
	Path    string `koanf:"PATH"`
	// MsgTypeAllowlist names the app-level msg_type values that get their own
	// metric label. Anything not listed is reported as "other". Empty (the
	// default) collapses every app-level msg_type — msg_type is
	// client-controlled, so this list is what bounds label cardinality.
	MsgTypeAllowlist []string `koanf:"MSG_TYPE_ALLOWLIST"`
}

// maxMetricsMsgTypeLen must stay in sync with metrics.maxMsgTypeLen.
const maxMetricsMsgTypeLen = 32

func (m *MetricsT) validate() {
	if !m.Enabled {
		return
	}
	if m.Port <= 0 || m.Port > 65535 {
		panic("METRICS.PORT must be between 1 and 65535 when METRICS.ENABLED is true")
	}
	if !strings.HasPrefix(m.Path, "/") {
		panic("METRICS.PATH must start with '/' when METRICS.ENABLED is true")
	}
	for _, e := range m.MsgTypeAllowlist {
		if e == "" || len(e) > maxMetricsMsgTypeLen {
			panic("METRICS.MSG_TYPE_ALLOWLIST entries must be 1..32 characters")
		}
		for i := 0; i < len(e); i++ {
			if e[i] < 0x20 || e[i] > 0x7E {
				panic("METRICS.MSG_TYPE_ALLOWLIST entries must be printable ASCII")
			}
		}
	}
}

func (m *MetricsT) loadDefault() {
	if m.Port == 0 {
		m.Port = 9090
	}
	if m.Path == "" {
		m.Path = "/metrics"
	}
}

// ValidateForTest exposes validate() to the types_test package.
func (m *MetricsT) ValidateForTest() { m.validate() }

// LoadDefaultForTest exposes loadDefault() to the types_test package.
func (m *MetricsT) LoadDefaultForTest() { m.loadDefault() }
```

Ensure `"strings"` is in the `export/types/config_child.go` import block. The
file currently imports `regexp`, `slices`, `time`, and `github.com/samber/lo` —
add `"strings"` alphabetically before `"time"`.

- [ ] **Step 4: Wire into EnvType**

In `export/types/config.go`, add the field after the `Otel` line (line 20):

```go
	Otel     *OtelT         `koanf:"OTEL"`
	Metrics  *MetricsT      `koanf:"METRICS"`
	Valkey   *ValkeyT       `koanf:"VALKEY"`
```

In `EnvType.Validate()`, after `e.Otel.validate()`:

```go
	e.Otel.validate()
	if e.Metrics != nil {
		e.Metrics.validate()
	}
```

In `EnvType.LoadDefault()`, after `e.Otel.loadDefault()`:

```go
	e.Otel.loadDefault()
	if e.Metrics == nil {
		e.Metrics = &MetricsT{}
	}
	e.Metrics.loadDefault()
```

The nil guards matter: existing YAML configs have no `METRICS` block, so koanf
leaves the pointer nil. `LoadDefault` allocates it so `cfg.Env().Metrics` is
always safe to dereference.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./export/types/... -run TestMetricsT -v`
Expected: PASS.

- [ ] **Step 6: Verify existing config loading still works**

Run: `go test ./export/... ./server/config/... -v`
Expected: PASS — no regression from the new pointer field.

- [ ] **Step 7: Commit**

```bash
git add export/types/config_child.go export/types/config.go export/types/metrics_config_test.go
git commit -m "feat(config): add METRICS block (opt-in, port 9090, msg_type allowlist)"
```

---

## Task 3: Tier 1 instruments

**Files:**
- Modify (rewrite): `pkg/metrics/metrics.go`
- Create: `pkg/metrics/metrics_test.go`
- Modify: `core/delivery/module/0.0.new.go` (remove `metrics.New()` call and field)

**Interfaces:**
- Consumes: `SanitizeMsgType`, `BuildAllowlist` (Task 1); `types.MetricsT` (Task 2).
- Produces:
  - `type Config struct { MsgTypeAllowlist []string; Version string; ContainerID string }`
  - `func New(cfg Config) *PipewaveMetrics`
  - `func (m *PipewaveMetrics) RecordConnectionAccepted(ctx context.Context, transport, auth string)`
  - `func (m *PipewaveMetrics) RecordConnectionRejected(ctx context.Context, transport, reason string)`
  - `func (m *PipewaveMetrics) RecordConnectionDuration(ctx context.Context, seconds float64, auth string)`
  - `func (m *PipewaveMetrics) RecordClientMessage(ctx context.Context, rawMsgType, outcome string, seconds float64)`
  - Reason constants: `RejectMissingToken`, `RejectInvalidToken`, `RejectUpgradeFailed`, `RejectRegisterFailed`
  - Outcome constants: `OutcomeOK`, `OutcomeError`, `OutcomeInvalidSchema`, `OutcomeDedup`, `OutcomeRateLimited`
  - Auth constants: `AuthAnon`, `AuthUser`; transport constants: `TransportWS`, `TransportLongPoll`

**Background — what is wrong with the current file.** `pkg/metrics/metrics.go`
today calls `prometheus.New()` and `otel.SetMeterProvider()` inside `New()` and
`panic`s on error. That hijacks the global provider of any host application that
embeds pipewave. Replace the whole file: read `otel.GetMeterProvider()`, never
set it, never panic.

Also note `activeConnections` is currently an `Int64UpDownCounter`. Do NOT keep
it — a single missed decrement (panic, crash, cancelled ctx) makes it drift
permanently. Active-connection gauges arrive in Task 5 as ObservableGauges that
read live state each scrape.

- [ ] **Step 1: Write the failing test**

Create `pkg/metrics/metrics_test.go`:

```go
package metrics_test

import (
	"context"
	"testing"

	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newTestReader installs a fresh MeterProvider backed by a ManualReader and
// restores the previous global provider when the test ends.
func newTestReader(t *testing.T) *metric.ManualReader {
	t.Helper()
	reader := metric.NewManualReader()
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(metric.NewMeterProvider(metric.WithReader(reader)))
	t.Cleanup(func() { otel.SetMeterProvider(prev) })
	return reader
}

func collect(t *testing.T, r *metric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, r.Collect(context.Background(), &rm))
	return rm
}

// findMetric returns the named metric from any scope, failing the test if absent.
func findMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Metrics{}
}

func sumFor(t *testing.T, m metricdata.Metrics, want map[string]string) int64 {
	t.Helper()
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "metric %s is not an int64 sum", m.Name)
	for _, dp := range sum.DataPoints {
		matched := true
		for k, v := range want {
			got, found := dp.Attributes.Value(attribute.Key(k))
			if !found || got.AsString() != v {
				matched = false
				break
			}
		}
		if matched {
			return dp.Value
		}
	}
	t.Fatalf("no datapoint on %s matching %v", m.Name, want)
	return 0
}

func TestRecordConnectionAccepted(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{Version: "v1", ContainerID: "c1"})

	m.RecordConnectionAccepted(context.Background(), metrics.TransportWS, metrics.AuthUser)
	m.RecordConnectionAccepted(context.Background(), metrics.TransportWS, metrics.AuthUser)
	m.RecordConnectionAccepted(context.Background(), metrics.TransportLongPoll, metrics.AuthAnon)

	got := findMetric(t, collect(t, reader), "pipewave_connections_accepted_total")
	require.Equal(t, int64(2), sumFor(t, got, map[string]string{"transport": "ws", "auth": "user"}))
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{"transport": "longpoll", "auth": "anon"}))
}

func TestRecordConnectionRejected(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	m.RecordConnectionRejected(context.Background(), metrics.TransportWS, metrics.RejectInvalidToken)

	got := findMetric(t, collect(t, reader), "pipewave_connections_rejected_total")
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{"transport": "ws", "reason": "invalid_token"}))
}

func TestRecordClientMessage_SanitizesMsgType(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{MsgTypeAllowlist: []string{"CHAT"}})

	// allowlisted -> kept
	m.RecordClientMessage(context.Background(), "CHAT", metrics.OutcomeOK, 0.01)
	// not allowlisted -> "other"
	m.RecordClientMessage(context.Background(), "SECRET", metrics.OutcomeOK, 0.02)
	// system heartbeat byte -> "heartbeat"
	m.RecordClientMessage(context.Background(), string([]byte{202}), metrics.OutcomeOK, 0.03)

	got := findMetric(t, collect(t, reader), "pipewave_client_messages_total")
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{"msg_type": "CHAT"}))
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{"msg_type": "other"}))
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{"msg_type": "heartbeat"}))
}

func TestRecordClientMessage_RecordsHistogram(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	m.RecordClientMessage(context.Background(), string([]byte{202}), metrics.OutcomeOK, 0.25)

	got := findMetric(t, collect(t, reader), "pipewave_client_message_duration_seconds")
	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, hist.DataPoints, 1)
	require.Equal(t, uint64(1), hist.DataPoints[0].Count)
	require.InDelta(t, 0.25, hist.DataPoints[0].Sum, 0.0001)
}

func TestRecordConnectionDuration(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	m.RecordConnectionDuration(context.Background(), 12.5, metrics.AuthUser)

	got := findMetric(t, collect(t, reader), "pipewave_connection_duration_seconds")
	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.InDelta(t, 12.5, hist.DataPoints[0].Sum, 0.0001)
}

func TestBuildInfo(t *testing.T) {
	reader := newTestReader(t)
	_ = metrics.New(metrics.Config{Version: "v0.0.1", ContainerID: "abc123"})

	got := findMetric(t, collect(t, reader), "pipewave_build_info")
	gauge, ok := got.Data.(metricdata.Gauge[int64])
	require.True(t, ok)
	require.Len(t, gauge.DataPoints, 1)
	require.Equal(t, int64(1), gauge.DataPoints[0].Value)
	v, found := gauge.DataPoints[0].Attributes.Value("version")
	require.True(t, found)
	require.Equal(t, "v0.0.1", v.AsString())
}

// A no-op global provider must not panic and must not require any config.
func TestNew_NoProviderIsSafe(t *testing.T) {
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(noop.NewMeterProvider())
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	m := metrics.New(metrics.Config{})
	require.NotPanics(t, func() {
		m.RecordConnectionAccepted(context.Background(), metrics.TransportWS, metrics.AuthUser)
		m.RecordConnectionRejected(context.Background(), metrics.TransportWS, metrics.RejectMissingToken)
		m.RecordConnectionDuration(context.Background(), 1, metrics.AuthUser)
		m.RecordClientMessage(context.Background(), "CHAT", metrics.OutcomeOK, 1)
	})
}
```

Add this import to the test file for the last test:
`"go.opentelemetry.io/otel/metric/noop"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/metrics/... -run 'TestRecord|TestBuildInfo|TestNew_NoProvider' -v`
Expected: FAIL — `metrics.New` signature mismatch / undefined constants.

- [ ] **Step 3: Rewrite pkg/metrics/metrics.go**

Replace the entire file contents:

```go
package metrics

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// meterName scopes every pipewave instrument.
const meterName = "github.com/pipewave-dev/go-pkg"

// Label values. Each dimension is a closed set so cardinality stays bounded.
const (
	TransportWS       = "ws"
	TransportLongPoll = "longpoll"

	AuthAnon = "anon"
	AuthUser = "user"

	RejectMissingToken   = "missing_token"
	RejectInvalidToken   = "invalid_token"
	RejectUpgradeFailed  = "upgrade_failed"
	RejectRegisterFailed = "register_failed"

	OutcomeOK            = "ok"
	OutcomeError         = "error"
	OutcomeInvalidSchema = "invalid_schema"
	OutcomeDedup         = "dedup"
	OutcomeRateLimited   = "rate_limited"
)

// Config carries the process-level values needed to build instruments.
type Config struct {
	// MsgTypeAllowlist bounds the msg_type label; see SanitizeMsgType.
	MsgTypeAllowlist []string
	Version          string
	ContainerID      string
}

// PipewaveMetrics holds the Tier 1 instruments: connection lifecycle and
// inbound client messages.
type PipewaveMetrics struct {
	connAccepted metric.Int64Counter
	connRejected metric.Int64Counter
	connDuration metric.Float64Histogram
	clientMsgs   metric.Int64Counter
	clientMsgDur metric.Float64Histogram

	msgTypeAllowlist map[string]struct{}
}

// New builds the Tier 1 instruments from the global MeterProvider.
//
// It deliberately does NOT create or install a MeterProvider: the embedding
// process owns that choice. With no provider configured the OTEL API hands back
// no-op instruments, so this is safe and free to call unconditionally.
//
// Instrument creation errors are logged and downgraded to no-op instruments —
// metrics must never take down the main path.
func New(cfg Config) *PipewaveMetrics {
	meter := otel.GetMeterProvider().Meter(meterName)

	m := &PipewaveMetrics{
		msgTypeAllowlist: BuildAllowlist(cfg.MsgTypeAllowlist),
	}

	m.connAccepted = mustCounter(meter, "pipewave_connections_accepted_total",
		"Total WebSocket/long-poll connections accepted")
	m.connRejected = mustCounter(meter, "pipewave_connections_rejected_total",
		"Total connection attempts rejected, by reason")
	m.clientMsgs = mustCounter(meter, "pipewave_client_messages_total",
		"Total inbound client messages, by type and outcome")

	m.connDuration = mustHistogram(meter, "pipewave_connection_duration_seconds",
		"Lifetime of a client connection in seconds")
	m.clientMsgDur = mustHistogram(meter, "pipewave_client_message_duration_seconds",
		"Time spent handling one inbound client message")

	m.registerBuildInfo(meter, cfg)

	return m
}

// registerBuildInfo publishes a constant 1 carrying version/container_id.
// container_id is deliberately confined to this metric: as a label on a counter
// or histogram it would multiply every series by the number of pods.
func (m *PipewaveMetrics) registerBuildInfo(meter metric.Meter, cfg Config) {
	g, err := meter.Int64ObservableGauge("pipewave_build_info",
		metric.WithDescription("Always 1; labels carry build and container identity"))
	if err != nil {
		slog.Warn("metrics: create pipewave_build_info failed", slog.Any("error", err))
		return
	}
	attrs := metric.WithAttributes(
		attribute.String("version", cfg.Version),
		attribute.String("container_id", cfg.ContainerID),
	)
	if _, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(g, 1, attrs)
		return nil
	}, g); err != nil {
		slog.Warn("metrics: register pipewave_build_info callback failed", slog.Any("error", err))
	}
}

// mustCounter never fails: on error it logs and returns a no-op instrument.
func mustCounter(meter metric.Meter, name, desc string) metric.Int64Counter {
	c, err := meter.Int64Counter(name, metric.WithDescription(desc))
	if err != nil {
		slog.Warn("metrics: create counter failed", slog.String("name", name), slog.Any("error", err))
		c, _ = noop.NewMeterProvider().Meter(meterName).Int64Counter(name)
	}
	return c
}

// mustHistogram never fails: on error it logs and returns a no-op instrument.
func mustHistogram(meter metric.Meter, name, desc string) metric.Float64Histogram {
	h, err := meter.Float64Histogram(name,
		metric.WithDescription(desc),
		metric.WithUnit("s"))
	if err != nil {
		slog.Warn("metrics: create histogram failed", slog.String("name", name), slog.Any("error", err))
		h, _ = noop.NewMeterProvider().Meter(meterName).Float64Histogram(name)
	}
	return h
}

// RecordConnectionAccepted counts one admitted connection.
func (m *PipewaveMetrics) RecordConnectionAccepted(ctx context.Context, transport, auth string) {
	m.connAccepted.Add(ctx, 1, metric.WithAttributes(
		attribute.String("transport", transport),
		attribute.String("auth", auth),
	))
}

// RecordConnectionRejected counts one refused connection attempt. reason must
// be one of the Reject* constants.
func (m *PipewaveMetrics) RecordConnectionRejected(ctx context.Context, transport, reason string) {
	m.connRejected.Add(ctx, 1, metric.WithAttributes(
		attribute.String("transport", transport),
		attribute.String("reason", reason),
	))
}

// RecordConnectionDuration records how long a connection stayed open.
func (m *PipewaveMetrics) RecordConnectionDuration(ctx context.Context, seconds float64, auth string) {
	m.connDuration.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("auth", auth),
	))
}

// RecordClientMessage counts one inbound message and records its handling time.
// rawMsgType is the client-supplied wire value and is sanitized here, so
// callers may pass it through unmodified.
func (m *PipewaveMetrics) RecordClientMessage(ctx context.Context, rawMsgType, outcome string, seconds float64) {
	msgType := SanitizeMsgType(rawMsgType, m.msgTypeAllowlist)
	m.clientMsgs.Add(ctx, 1, metric.WithAttributes(
		attribute.String("msg_type", msgType),
		attribute.String("outcome", outcome),
	))
	m.clientMsgDur.Record(ctx, seconds, metric.WithAttributes(
		attribute.String("msg_type", msgType),
	))
}
```

- [ ] **Step 4: Remove the dead wiring from moduleDelivery**

`core/delivery/module/0.0.new.go` constructs `metrics.New()` but never records
anything. Ownership moves to DI in Task 4, so remove it here:

- Delete the import line `"github.com/pipewave-dev/go-pkg/pkg/metrics"`.
- Delete `metrics:       metrics.New(),` from the struct literal (line 31).
- Delete the field `metrics       *metrics.PipewaveMetrics` (line 59).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/metrics/... -v`
Expected: PASS.

Run: `go build ./...`
Expected: builds cleanly (confirms the moduleDelivery edit compiles).

- [ ] **Step 6: Lint**

Run: `golangci-lint run ./pkg/metrics/... ./core/delivery/...`
Expected: no findings.

- [ ] **Step 7: Commit**

```bash
git add pkg/metrics/metrics.go pkg/metrics/metrics_test.go core/delivery/module/0.0.new.go
git commit -m "feat(metrics): Tier 1 connection and message instruments

Reads the global MeterProvider instead of installing one, so embedders keep
ownership of their registry. Drops the UpDownCounter for active connections
(permanent drift on a missed decrement) and the panic on instrument error."
```

---

## Task 4: Metrics provider (exporter + listener)

**Files:**
- Create: `provider/metrics-provider/metrics.go`
- Create: `provider/metrics-provider/metrics_test.go`
- Modify: `do_packages.go`

**Interfaces:**
- Consumes: `types.MetricsT` (Task 2); `metrics.New`, `metrics.Config` (Task 3).
- Produces:
  - `func NewDI(i do.Injector) (*Provider, error)` — registered in the DI graph
  - `func (p *Provider) Metrics() *metrics.PipewaveMetrics`
  - `func (p *Provider) Handler() http.Handler` — returns nil when disabled
  - `func (p *Provider) ListenAndServe() error` — no-op returning nil when disabled
  - `func (p *Provider) Shutdown(ctx context.Context) error`

**Background:** Follow `provider/otel-provider/otel.go` for shape: pull
`configprovider.ConfigStore` and `fncollector.CleanupTask` from the injector,
register a cleanup task. Unlike otel-provider, this one **must** call
`otel.SetMeterProvider()` — the container is the process owner, so installing
the global provider here is correct. `pkg/metrics` (library side) still never does.

The Prometheus exporter (`go.opentelemetry.io/otel/exporters/prometheus`)
registers into `prometheus.DefaultRegisterer` by default. Serve it with
`promhttp.Handler()`.

- [ ] **Step 1: Write the failing test**

Create `provider/metrics-provider/metrics_test.go`:

```go
package metricsprovider_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pipewave-dev/go-pkg/export/types"
	metricsprovider "github.com/pipewave-dev/go-pkg/provider/metrics-provider"
	"github.com/stretchr/testify/require"
)

func TestProvider_DisabledIsInert(t *testing.T) {
	p, err := metricsprovider.NewForTest(&types.MetricsT{Enabled: false})
	require.NoError(t, err)
	require.Nil(t, p.Handler())
	require.NoError(t, p.ListenAndServe()) // must not block, must not error
	require.NotNil(t, p.Metrics())         // still returns usable no-op metrics
	require.NoError(t, p.Shutdown(context.Background()))
}

func TestProvider_EnabledServesMetrics(t *testing.T) {
	p, err := metricsprovider.NewForTest(&types.MetricsT{
		Enabled: true, Port: 0, Path: "/metrics",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	// Record something so at least one pipewave series exists.
	p.Metrics().RecordConnectionAccepted(context.Background(), "ws", "user")

	srv := httptest.NewServer(p.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "pipewave_connections_accepted_total")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./provider/metrics-provider/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the provider**

Create `provider/metrics-provider/metrics.go`:

```go
// Package metricsprovider owns the process-level metrics pipeline for the
// pipewave container: a Prometheus exporter, the global MeterProvider, and a
// dedicated HTTP listener serving /metrics.
//
// Installing the global MeterProvider is correct HERE because the container
// owns the process. pkg/metrics (used by the embeddable library) only ever
// reads the global provider, so a Go host embedding pipewave keeps control of
// its own registry.
package metricsprovider

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/pipewave-dev/go-pkg/export/types"
	"github.com/pipewave-dev/go-pkg/global/constants"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/samber/do/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Provider owns the metrics exporter, MeterProvider and listener.
type Provider struct {
	cfg      *types.MetricsT
	metrics  *metrics.PipewaveMetrics
	mp       *sdkmetric.MeterProvider
	handler  http.Handler
	srv      *http.Server
	listener net.Listener
}

// NewDI builds the provider from the injector and registers a shutdown task.
func NewDI(i do.Injector) (*Provider, error) {
	cfg := do.MustInvoke[configprovider.ConfigStore](i)
	cleanupTask := do.MustInvoke[fncollector.CleanupTask](i)

	env := cfg.Env()
	p, err := newProvider(env.Metrics, env.Info.ContainerID)
	if err != nil {
		return nil, err
	}

	cleanupTask.RegTask(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.Shutdown(ctx); err != nil {
			slog.Warn("metrics: shutdown failed", slog.Any("error", err))
		}
	}, fncollector.FnPriorityNormal)

	return p, nil
}

// NewForTest builds a provider without the DI graph. Test-only.
func NewForTest(cfg *types.MetricsT) (*Provider, error) {
	return newProvider(cfg, "test-container")
}

func newProvider(cfg *types.MetricsT, containerID string) (*Provider, error) {
	if cfg == nil {
		cfg = &types.MetricsT{}
	}
	p := &Provider{cfg: cfg}

	if !cfg.Enabled {
		// Do not touch the global provider. metrics.New falls back to the
		// no-op API, so every Record* call is free.
		p.metrics = metrics.New(metrics.Config{})
		return p, nil
	}

	exporter, err := prometheus.New()
	if err != nil {
		// Metrics must never stop the server from booting.
		slog.Error("metrics: prometheus exporter init failed; metrics disabled",
			slog.Any("error", err))
		p.cfg = &types.MetricsT{}
		p.metrics = metrics.New(metrics.Config{})
		return p, nil
	}

	p.mp = sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(p.mp)

	p.metrics = metrics.New(metrics.Config{
		MsgTypeAllowlist: cfg.MsgTypeAllowlist,
		Version:          constants.Version,
		ContainerID:      containerID,
	})

	mux := http.NewServeMux()
	mux.Handle(cfg.Path, promhttp.Handler())
	p.handler = mux

	return p, nil
}

// Metrics returns the instrument set. Never nil.
func (p *Provider) Metrics() *metrics.PipewaveMetrics { return p.metrics }

// Handler returns the /metrics handler, or nil when metrics are disabled.
func (p *Provider) Handler() http.Handler { return p.handler }

// ListenAndServe starts the metrics listener and blocks until shutdown.
// Returns nil immediately when metrics are disabled.
//
// Callers should run this in a goroutine and log (not fatal) on error: a
// metrics listener that fails to bind must not take the server down.
func (p *Provider) ListenAndServe() error {
	if p.handler == nil {
		return nil
	}
	addr := ":" + strconv.Itoa(p.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	p.listener = ln
	p.srv = &http.Server{Handler: p.handler, ReadHeaderTimeout: 5 * time.Second}
	slog.Info("metrics: listening", slog.String("addr", ln.Addr().String()),
		slog.String("path", p.cfg.Path))
	if err := p.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the listener and flushes the MeterProvider.
func (p *Provider) Shutdown(ctx context.Context) error {
	var firstErr error
	if p.srv != nil {
		if err := p.srv.Shutdown(ctx); err != nil {
			firstErr = err
		}
	}
	if p.mp != nil {
		if err := p.mp.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
```

- [ ] **Step 4: Add the promhttp dependency**

Run: `go get github.com/prometheus/client_golang/prometheus/promhttp`

`github.com/prometheus/client_golang` is already an indirect dependency of the
OTEL Prometheus exporter; this promotes it to direct.

- [ ] **Step 5: Register in the DI graph**

In `do_packages.go`, add the import (alphabetically among providers):

```go
	metricsprovider "github.com/pipewave-dev/go-pkg/provider/metrics-provider"
```

and add to the `do.Package(...)` list, after `do.Lazy(healthyprovider.NewDI),`:

```go
		do.Lazy(metricsprovider.NewDI),
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./provider/metrics-provider/... -v`
Expected: PASS both tests.

Run: `go build ./... && go test ./export/... ./pkg/metrics/... -v`
Expected: PASS.

- [ ] **Step 7: Lint**

Run: `golangci-lint run ./provider/metrics-provider/...`
Expected: no findings.

- [ ] **Step 8: Commit**

```bash
git add provider/metrics-provider/ do_packages.go go.mod go.sum
git commit -m "feat(metrics): provider owning exporter, MeterProvider and :9090 listener"
```

---

## Task 5: Active-connection ObservableGauges

**Files:**
- Create: `pkg/metrics/observable.go`
- Create: `pkg/metrics/observable_test.go`

**Interfaces:**
- Consumes: `meterName`, `mustCounter` conventions from Task 3.
- Produces:
  - `type ConnectionStatsSource interface { InsideActiveConnection(ctx context.Context) (*business.SumaryActiveConnection, aerror.AError) }`
  - `func (m *PipewaveMetrics) RegisterConnectionGauges(src ConnectionStatsSource) error`

**Background — why ObservableGauge.** `business.Monitoring.InsideActiveConnection`
(`core/service/business/monitoring.go:32`) returns
`*SumaryActiveConnection{AnonymosConnection, UserConnection, TotalUser}` (note
the existing typo `Anonymos` — keep it, it is the real field name). The impl in
`core/service/business/monitoring/` reads **in-memory** maps from
`connManager`, so it is cheap and does not touch the database.

Report **per-container** counts (`InsideActiveConnection`), never
`TotalActiveConnection`: every pod exports its own share and Prometheus `sum()`
aggregates. Using the cluster-wide total would multiply the real number by the
pod count.

Even though the source is in-memory today, wrap the callback in a short timeout
and cache the last good value — the callback runs on the scrape path, and a
future implementation change must not be able to stall scrapes.

- [ ] **Step 1: Write the failing test**

Create `pkg/metrics/observable_test.go`:

```go
package metrics_test

import (
	"context"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/core/service/business"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type stubStats struct {
	sum   *business.SumaryActiveConnection
	err   aerror.AError
	delay time.Duration
	calls int
}

func (s *stubStats) InsideActiveConnection(ctx context.Context) (*business.SumaryActiveConnection, aerror.AError) {
	s.calls++
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, aerror.New(ctx, aerror.ErrUnexpectedBussiness, ctx.Err())
		}
	}
	return s.sum, s.err
}

func gaugeFor(t *testing.T, m metricdata.Metrics, want map[string]string) int64 {
	t.Helper()
	g, ok := m.Data.(metricdata.Gauge[int64])
	require.True(t, ok, "metric %s is not an int64 gauge", m.Name)
	for _, dp := range g.DataPoints {
		matched := true
		for k, v := range want {
			got, found := dp.Attributes.Value(attribute.Key(k))
			if !found || got.AsString() != v {
				matched = false
				break
			}
		}
		if matched {
			return dp.Value
		}
	}
	t.Fatalf("no datapoint on %s matching %v", m.Name, want)
	return 0
}

func TestRegisterConnectionGauges(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	src := &stubStats{sum: &business.SumaryActiveConnection{
		AnonymosConnection: 3, UserConnection: 7, TotalUser: 5,
	}}
	require.NoError(t, m.RegisterConnectionGauges(src))

	rm := collect(t, reader)
	active := findMetric(t, rm, "pipewave_connections_active")
	require.Equal(t, int64(3), gaugeFor(t, active, map[string]string{"auth": "anon"}))
	require.Equal(t, int64(7), gaugeFor(t, active, map[string]string{"auth": "user"}))

	users := findMetric(t, rm, "pipewave_users_active")
	require.Equal(t, int64(5), gaugeFor(t, users, nil))
}

func TestRegisterConnectionGauges_ReflectsChanges(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	src := &stubStats{sum: &business.SumaryActiveConnection{AnonymosConnection: 1, UserConnection: 1, TotalUser: 1}}
	require.NoError(t, m.RegisterConnectionGauges(src))
	_ = collect(t, reader)

	// Gauges read live state each scrape, so a change must be visible without
	// any Record* call — this is what makes them drift-free.
	src.sum = &business.SumaryActiveConnection{AnonymosConnection: 42, UserConnection: 0, TotalUser: 9}
	active := findMetric(t, collect(t, reader), "pipewave_connections_active")
	require.Equal(t, int64(42), gaugeFor(t, active, map[string]string{"auth": "anon"}))
}

func TestRegisterConnectionGauges_ErrorKeepsLastGoodValue(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	src := &stubStats{sum: &business.SumaryActiveConnection{AnonymosConnection: 5, UserConnection: 5, TotalUser: 5}}
	require.NoError(t, m.RegisterConnectionGauges(src))
	_ = collect(t, reader) // primes the cache

	src.err = aerror.New(context.Background(), aerror.ErrUnexpectedBussiness, nil)
	src.sum = nil

	active := findMetric(t, collect(t, reader), "pipewave_connections_active")
	require.Equal(t, int64(5), gaugeFor(t, active, map[string]string{"auth": "anon"}))
}

func TestRegisterConnectionGauges_SlowSourceDoesNotBlockScrape(t *testing.T) {
	reader := newTestReader(t)
	m := metrics.New(metrics.Config{})

	src := &stubStats{
		sum:   &business.SumaryActiveConnection{AnonymosConnection: 1, UserConnection: 1, TotalUser: 1},
		delay: 3 * time.Second,
	}
	require.NoError(t, m.RegisterConnectionGauges(src))

	start := time.Now()
	_ = collect(t, reader)
	elapsed := time.Since(start)

	// The source sleeps 3s; the callback's own timeout is 2s. Assert it bailed
	// out on the timeout rather than waiting for the source: comfortably under
	// the source delay, but not instant (which would mean the timeout never
	// engaged and something else returned early).
	//
	// Do NOT assert `elapsed < statsCallbackTimeout` — reaching the timeout
	// consumes the whole 2s before any post-cancellation work runs, so that
	// bound is unsatisfiable by construction.
	require.Less(t, elapsed, 3*time.Second,
		"callback must bail out on its own timeout, not wait for the slow source")
	require.GreaterOrEqual(t, elapsed, metrics.StatsCallbackTimeoutForTest(),
		"callback returned before its timeout could have fired")
}
```

`aerror.ErrUnexpectedBussiness` is the real identifier (note the existing
misspelling) — verified in `shared/aerror/0_error_code.go:119`. There is no
plain `aerror.ErrUnexpected`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/metrics/... -run TestRegisterConnectionGauges -v`
Expected: FAIL — `m.RegisterConnectionGauges` undefined.

- [ ] **Step 3: Write the implementation**

Create `pkg/metrics/observable.go`:

```go
package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pipewave-dev/go-pkg/core/service/business"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// statsCallbackTimeout bounds how long a scrape waits on the stats source.
// The callback runs on the Prometheus scrape path; without a bound a slow
// source would let scrapes pile up.
const statsCallbackTimeout = 2 * time.Second

// StatsCallbackTimeoutForTest exposes statsCallbackTimeout to the metrics_test
// package, so the timing test derives its bound from the real constant instead
// of hardcoding it.
func StatsCallbackTimeoutForTest() time.Duration { return statsCallbackTimeout }

// ConnectionStatsSource supplies live per-container connection counts.
// business.Monitoring satisfies it.
type ConnectionStatsSource interface {
	InsideActiveConnection(ctx context.Context) (*business.SumaryActiveConnection, aerror.AError)
}

// connStatsCache holds the last successful reading so a transient failure
// reports stale-but-plausible numbers instead of a gap in the series.
type connStatsCache struct {
	mu    sync.Mutex
	value *business.SumaryActiveConnection
}

func (c *connStatsCache) get() *business.SumaryActiveConnection {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

func (c *connStatsCache) set(v *business.SumaryActiveConnection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = v
}

// RegisterConnectionGauges publishes pipewave_connections_active and
// pipewave_users_active, read from src on every scrape.
//
// These are ObservableGauges, not UpDownCounters, on purpose: a counter that is
// incremented on connect and decremented on disconnect drifts permanently the
// first time a decrement is missed (panic, crash, cancelled context). Reading
// live state each scrape is self-correcting.
//
// The values are per-container (InsideActiveConnection). Each pod reports its
// own share and Prometheus sum() aggregates; using a cluster-wide total here
// would over-count by the number of pods.
func (m *PipewaveMetrics) RegisterConnectionGauges(src ConnectionStatsSource) error {
	meter := otel.GetMeterProvider().Meter(meterName)

	active, err := meter.Int64ObservableGauge("pipewave_connections_active",
		metric.WithDescription("Active connections held by this container, by auth kind"))
	if err != nil {
		return err
	}
	users, err := meter.Int64ObservableGauge("pipewave_users_active",
		metric.WithDescription("Distinct users connected to this container"))
	if err != nil {
		return err
	}

	cache := &connStatsCache{}
	anonAttrs := metric.WithAttributes(attribute.String("auth", AuthAnon))
	userAttrs := metric.WithAttributes(attribute.String("auth", AuthUser))

	_, err = meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		ctx, cancel := context.WithTimeout(ctx, statsCallbackTimeout)
		defer cancel()

		sum, aErr := src.InsideActiveConnection(ctx)
		if aErr != nil || sum == nil {
			// Fall back to the previous reading; a failed scrape must not
			// surface as a zero, which would look like an outage.
			sum = cache.get()
			if sum == nil {
				slog.Warn("metrics: connection stats unavailable and no cached value",
					slog.Any("error", aErr))
				return nil
			}
		} else {
			cache.set(sum)
		}

		o.ObserveInt64(active, int64(sum.AnonymosConnection), anonAttrs)
		o.ObserveInt64(active, int64(sum.UserConnection), userAttrs)
		o.ObserveInt64(users, int64(sum.TotalUser))
		return nil
	}, active, users)

	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/metrics/... -run TestRegisterConnectionGauges -v`
Expected: PASS all four.

- [ ] **Step 5: Run the whole metrics package**

Run: `go test ./pkg/metrics/... -v`
Expected: PASS.

- [ ] **Step 6: Lint**

Run: `golangci-lint run ./pkg/metrics/...`
Expected: no findings.

- [ ] **Step 7: Commit**

```bash
git add pkg/metrics/observable.go pkg/metrics/observable_test.go
git commit -m "feat(metrics): drift-free active-connection gauges via ObservableGauge"
```

---

## Task 6: Instrument connection accept/reject/duration

**Files:**
- Modify: `core/service/websocket/mediator/delivery/0.new.go`
- Modify: `core/service/websocket/mediator/delivery/2.gobwas_endpoint.go`
- Test: `core/service/websocket/mediator/delivery/conn_metrics_test.go`

**Interfaces:**
- Consumes: `*metrics.PipewaveMetrics` and its `RecordConnectionAccepted`, `RecordConnectionRejected`, `RecordConnectionDuration`, plus `TransportWS`, `AuthAnon`, `AuthUser`, `Reject*` constants (Task 3); `*metricsprovider.Provider` (Task 4).
- Produces: `func authKind(auth voAuth.WebsocketAuth) string` in package `delivery` (returns `metrics.AuthAnon` / `metrics.AuthUser`).

**Background — where the hooks are.** `serverDelivery.registerCallback()`
(`0.new.go:148`) holds both lifecycle hooks in one place: the open hook at line
149 (`d.onNewStuff.Register(...)`) and the close hook at line 236
(`d.onCloseStuff.RegisterAll(...)`). `WebsocketConn` has **no open timestamp**
(`core/service/websocket/2.connection_type.go`), so duration needs its own map
keyed the same way `authKey(auth)` already is (`userID + ":" + instanceID`).

Reject reasons map to the returns in `2.gobwas_endpoint.go`: line 24
`missing_token`, line 31 `invalid_token`, line 40 `upgrade_failed`, lines 48+58
`register_failed`.

**Do not** add a `rate_limited` reason on `/gw`: the IP limiter
(`ip_rate_limiter.go`) guards `POST /issue-tmp-token`, not the upgrade path.

`voAuth.WebsocketAuth` has an `IsAnonymous()` method (used at `0.new.go:244`).

- [ ] **Step 1: Write the failing test**

Create `core/service/websocket/mediator/delivery/conn_metrics_test.go`:

```go
package delivery

import (
	"testing"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/stretchr/testify/require"
)

func TestAuthKind(t *testing.T) {
	require.Equal(t, metrics.AuthUser, authKind(voAuth.UserWebsocketAuth("u1", "i1")))
	require.Equal(t, metrics.AuthAnon, authKind(voAuth.AnonymousUserWebsocketAuth("i1")))
}

func TestConnTracker_ReturnsElapsed(t *testing.T) {
	tr := newConnTracker()
	auth := voAuth.UserWebsocketAuth("u1", "i1")

	tr.open(auth)
	time.Sleep(10 * time.Millisecond)
	d, ok := tr.close(auth)
	require.True(t, ok)
	require.GreaterOrEqual(t, d, 10*time.Millisecond)
}

func TestConnTracker_CloseWithoutOpen(t *testing.T) {
	tr := newConnTracker()
	_, ok := tr.close(voAuth.UserWebsocketAuth("u1", "i1"))
	require.False(t, ok, "close without open must report not-found, not a bogus duration")
}

func TestConnTracker_CloseIsIdempotent(t *testing.T) {
	tr := newConnTracker()
	auth := voAuth.UserWebsocketAuth("u1", "i1")

	tr.open(auth)
	_, ok := tr.close(auth)
	require.True(t, ok)

	// Second close must not find an entry — otherwise the map leaks.
	_, ok = tr.close(auth)
	require.False(t, ok)
}

func TestConnTracker_DistinctSessions(t *testing.T) {
	tr := newConnTracker()
	a := voAuth.UserWebsocketAuth("u1", "i1")
	b := voAuth.UserWebsocketAuth("u1", "i2")

	tr.open(a)
	tr.open(b)
	_, okA := tr.close(a)
	_, okB := tr.close(b)
	require.True(t, okA)
	require.True(t, okB)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/service/websocket/mediator/delivery/... -run 'TestAuthKind|TestConnTracker' -v`
Expected: FAIL — `undefined: authKind`, `undefined: newConnTracker`.

- [ ] **Step 3: Add the tracker and helper**

Create `core/service/websocket/mediator/delivery/conn_metrics.go`:

```go
package delivery

import (
	"sync"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
)

// authKind maps an auth to the bounded "auth" metric label.
func authKind(auth voAuth.WebsocketAuth) string {
	if auth.IsAnonymous() {
		return metrics.AuthAnon
	}
	return metrics.AuthUser
}

// connTracker records connection open times so close can report a duration.
// WebsocketConn carries no open timestamp, so this is the only place it lives.
type connTracker struct {
	mu     sync.Mutex
	openAt map[string]time.Time
}

func newConnTracker() *connTracker {
	return &connTracker{openAt: make(map[string]time.Time)}
}

func (t *connTracker) open(auth voAuth.WebsocketAuth) {
	t.mu.Lock()
	t.openAt[authKey(auth)] = time.Now()
	t.mu.Unlock()
}

// close removes the entry and returns how long the connection was open.
// ok is false when no matching open was recorded, so callers skip the metric
// rather than reporting a duration measured from the zero time.
func (t *connTracker) close(auth voAuth.WebsocketAuth) (d time.Duration, ok bool) {
	key := authKey(auth)
	t.mu.Lock()
	start, found := t.openAt[key]
	if found {
		delete(t.openAt, key)
	}
	t.mu.Unlock()
	if !found {
		return 0, false
	}
	return time.Since(start), true
}
```

`authKey` already exists in this package? **No** — it is in
`core/service/websocket/ws-event-trigger/0.2.new_on_close.go:32`, a different
package. Add a local copy to `conn_metrics.go`:

```go
// authKey identifies a session; mirrors ws-event-trigger's keying.
func authKey(auth voAuth.WebsocketAuth) string {
	return auth.UserID + ":" + auth.InstanceID
}
```

Before adding it, run `grep -n "func authKey" core/service/websocket/mediator/delivery/*.go`
to confirm there is no existing definition in this package. If one exists, reuse it.

- [ ] **Step 4: Run tracker tests to verify they pass**

Run: `go test ./core/service/websocket/mediator/delivery/... -run 'TestAuthKind|TestConnTracker' -v`
Expected: PASS.

- [ ] **Step 5: Wire the tracker and metrics into serverDelivery**

In `0.new.go`, add to the imports:

```go
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	metricsprovider "github.com/pipewave-dev/go-pkg/provider/metrics-provider"
```

In `NewDI`, after the other `do.MustInvoke` lines (~line 37):

```go
	metricsProvider := do.MustInvoke[*metricsprovider.Provider](i)
```

Add to the `&serverDelivery{...}` literal:

```go
		metrics:     metricsProvider.Metrics(),
		connTracker: newConnTracker(),
```

Add to the `serverDelivery` struct (after `shutdownSignal`):

```go
	// metrics records connection lifecycle counters/histograms.
	metrics *metrics.PipewaveMetrics
	// connTracker holds open timestamps so close can report a duration.
	connTracker *connTracker
```

In `registerCallback()`, at the **end** of the `onNewStuff.Register` callback,
just before the final `return nil` (currently line 233):

```go
		d.connTracker.open(auth)
		d.metrics.RecordConnectionAccepted(ctx, metrics.TransportWS, authKind(auth))

		return nil
```

Record accept here, not in the HTTP handler: this is the point where the
connection is fully admitted (registered, drained, rate-limiter primed).

At the **start** of the `onCloseStuff.RegisterAll` callback (currently line 236,
before `d.connectionMgr.RemoveConnection(auth)`), write exactly this:

```go
	d.onCloseStuff.RegisterAll(func(auth voAuth.WebsocketAuth) {
		ctx := actx.New()
		ctx.SetWebsocketAuth(auth)

		if dur, ok := d.connTracker.close(auth); ok {
			d.metrics.RecordConnectionDuration(ctx, dur.Seconds(), authKind(auth))
		}

		d.connectionMgr.RemoveConnection(auth)
		d.rateLimiter.Remove(auth)
		// ... existing body continues, but DELETE the duplicated
		// ctx := actx.New() / ctx.SetWebsocketAuth(auth) lines that
		// previously appeared here (old lines 240-241).
```

The existing callback already builds `ctx` at old lines 240-241; move that
construction to the top as shown and delete the original two lines so `ctx` is
declared exactly once.

Name the duration variable `dur`, not `d` — `d` is the `*serverDelivery`
receiver, and shadowing it inside the callback would break every `d.` reference
in the rest of the body.

- [ ] **Step 6: Record rejections in the gobwas endpoint**

Rewrite `2.gobwas_endpoint.go` to record a reason on each failure path:

```go
package delivery

import (
	"log/slog"
	"net/http"

	"github.com/gobwas/ws"
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
)

// GobwasEndpoint handles /gw
// Upgrades HTTP connection to WebSocket using gobwas library
func (d *serverDelivery) GobwasEndpoint() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			auth voAuth.WebsocketAuth
			err  error
		)
		ctx := r.Context()

		// 1. Get connection token from query parameter
		connToken := r.URL.Query().Get("tk")
		switch connToken {
		case "":
			d.metrics.RecordConnectionRejected(ctx, metrics.TransportWS, metrics.RejectMissingToken)
			http.Error(w, "Missing connection token", http.StatusUnauthorized)
			return

		default:
			// Scan temporary connection token
			auth, err = d.exchangeToken.ScanConnToken(r.Context(), connToken)
			if err != nil {
				d.metrics.RecordConnectionRejected(ctx, metrics.TransportWS, metrics.RejectInvalidToken)
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
		}

		// 2. Upgrade HTTP connection to WebSocket
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			d.metrics.RecordConnectionRejected(ctx, metrics.TransportWS, metrics.RejectUpgradeFailed)
			slog.Warn("Failed to upgrade connection", slog.Any("error", err))
			http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
			return
		}

		// 3. Create WebSocket connection wrapper
		wsConn, aErr := d.gobwasServer.NewConnection(conn, auth)
		if aErr != nil {
			d.metrics.RecordConnectionRejected(ctx, metrics.TransportWS, metrics.RejectRegisterFailed)
			slog.Error("Failed to create WebSocket connection", slog.Any("error", aErr))
			http.Error(w, aErr.Error(), http.StatusInternalServerError)
			return
		}

		// 4. Handle new connection (register, persist to DynamoDB)
		if err := d.onNewStuff.Do(wsConn); err != nil {
			// wsConn was already registered with netpoll by NewConnection above;
			// close it here so the fd isn't leaked (it never reaches
			// ConnectionManager, so nothing else will clean it up).
			d.metrics.RecordConnectionRejected(ctx, metrics.TransportWS, metrics.RejectRegisterFailed)
			wsConn.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Connection is now active and will be handled by gobwas server
		slog.Debug("New WebSocket connection established",
			slog.Any("auth", auth),
			slog.String("remote_addr", r.RemoteAddr))
	})
}
```

- [ ] **Step 7: Build and run the package tests**

Run: `go build ./... && go test ./core/service/websocket/... -v`
Expected: PASS. If DI-based tests fail with a missing
`*metricsprovider.Provider`, the test injector needs
`do.Lazy(metricsprovider.NewDI)` — add it wherever that test builds its
injector.

- [ ] **Step 8: Lint**

Run: `golangci-lint run ./core/service/websocket/mediator/delivery/...`
Expected: no findings.

- [ ] **Step 9: Commit**

```bash
git add core/service/websocket/mediator/delivery/
git commit -m "feat(metrics): record connection accept, reject reason and duration"
```

---

## Task 7: Instrument inbound client messages

**Files:**
- Modify: `core/service/websocket/client-msg-handler/0_main_handler.go`
- Test: `core/service/websocket/client-msg-handler/msg_metrics_test.go`

**Interfaces:**
- Consumes: `*metrics.PipewaveMetrics`, `Outcome*` constants (Task 3); `*metricsprovider.Provider` (Task 4).
- Produces: nothing consumed by later tasks.

**Background — the branches to cover.** In `handleMessage`
(`0_main_handler.go:74`) the outcomes are:

| Condition | Outcome |
|---|---|
| `msg.Unmarshall` fails | `OutcomeInvalidSchema` |
| heartbeat/ack rate-limited (`rateLimiter.Get(auth).Allow()` false) | `OutcomeRateLimited` |
| default-branch rate-limited | `OutcomeRateLimited` |
| `deduplicator.isDuplicate` true | `OutcomeDedup` |
| `HandleMessage` returns err | `OutcomeError` |
| otherwise | `OutcomeOK` |

The cleanest instrumentation point is a single `defer` at the top of
`handleMessage` — it fires on every return path, so no branch can be missed.
Capture `msgType` and `outcome` in local variables the branches assign.

Note the unmarshal-failure path has no valid `msg.MsgType`, so it reports the
empty string, which `SanitizeMsgType` maps to `"other"`.

- [ ] **Step 1: Write the failing test**

Create `core/service/websocket/client-msg-handler/msg_metrics_test.go`:

```go
package clientmsghandler

import (
	"testing"

	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/stretchr/testify/require"
)

// outcomeRecorder is the seam the handler writes through.
func TestOutcomeConstantsAreDistinct(t *testing.T) {
	all := []string{
		metrics.OutcomeOK,
		metrics.OutcomeError,
		metrics.OutcomeInvalidSchema,
		metrics.OutcomeDedup,
		metrics.OutcomeRateLimited,
	}
	seen := make(map[string]struct{}, len(all))
	for _, o := range all {
		require.NotEmpty(t, o)
		_, dup := seen[o]
		require.False(t, dup, "duplicate outcome value %q", o)
		seen[o] = struct{}{}
	}
}
```

This is a thin guard; the real coverage for this task is the existing handler
test suite plus a manual smoke check in Step 6. A full unit test of
`handleMessage` requires constructing `clientMsgHandler` with six collaborators
(config, rate limiter, dedup, ack manager, repos, broadcast) — out of proportion
here, and the `defer` shape makes branch omission structurally impossible.

- [ ] **Step 2: Run test to verify it compiles and fails/passes**

Run: `go test ./core/service/websocket/client-msg-handler/... -run TestOutcomeConstants -v`
Expected: PASS once Task 3 is merged (constants exist). If it fails with
`undefined`, Task 3 was not completed.

- [ ] **Step 3: Add the metrics field**

In `0_main_handler.go`, add imports:

```go
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	metricsprovider "github.com/pipewave-dev/go-pkg/provider/metrics-provider"
```

In `NewDI`, add:

```go
	metricsProvider := do.MustInvoke[*metricsprovider.Provider](i)
```

and to the returned struct literal:

```go
		metrics:       metricsProvider.Metrics(),
```

and to the `clientMsgHandler` struct:

```go
	metrics       *metrics.PipewaveMetrics
```

- [ ] **Step 4: Instrument handleMessage**

Replace the opening of `handleMessage` (the `var response ...` line through the
end of the unmarshal error branch) with:

```go
func (h *clientMsgHandler) handleMessage(ctx context.Context, clientMsg []byte, auth voAuth.WebsocketAuth, sendFn func(context.Context, []byte) error) {
	var response *wsSv.WebsocketResponse

	// Instrument every exit path from one place: a defer cannot miss a branch
	// the way scattered Record calls can.
	start := time.Now()
	rawMsgType := ""
	outcome := metrics.OutcomeOK
	defer func() {
		h.metrics.RecordClientMessage(ctx, rawMsgType, outcome, time.Since(start).Seconds())
	}()

	defer func() {
		if response != nil {
			data := response.Marshall()
			sendFn(ctx, data)
		}
	}()

	var msg wsSv.WebsocketResquest
	err2 := msg.Unmarshall(clientMsg)
	if err2 != nil {
		// Invalid message format
		outcome = metrics.OutcomeInvalidSchema
		response = &wsSv.WebsocketResponse{
			Error: aerror.New(ctx, aerror.InvalidInputSchema, err2).Error(),
		}
		return
	}
	rawMsgType = string(msg.MsgType)
```

Defer order matters: the metrics defer is registered **first**, so it runs
**last** — after the response has been sent, so the recorded duration covers the
write too.

Then set `outcome` in each remaining branch:

In the `case wsSv.MessageTypeHeartbeat:` branch:

```go
	case wsSv.MessageTypeHeartbeat:
		if !h.rateLimiter.Get(auth).Allow() {
			outcome = metrics.OutcomeRateLimited
			return
		}
```

In the `case wsSv.MessageTypeAck:` branch:

```go
	case wsSv.MessageTypeAck:
		if !h.rateLimiter.Get(auth).Allow() {
			outcome = metrics.OutcomeRateLimited
			return
		}
```

In the `default:` branch, the rate-limit rejection:

```go
		if !rl.Allow() {
			outcome = metrics.OutcomeRateLimited
			response = &wsSv.WebsocketResponse{
```

the dedup return:

```go
		if msg.Id != "" && h.deduplicator.isDuplicate(msg.Id+auth.InstanceID) {
			outcome = metrics.OutcomeDedup
			return
		}
```

and the `HandleMessage` error:

```go
		if err != nil {
			outcome = metrics.OutcomeError
			response = &wsSv.WebsocketResponse{
```

Leave every other path at the `OutcomeOK` default.

- [ ] **Step 5: Build and test**

Run: `go build ./... && go test ./core/service/websocket/... -v`
Expected: PASS.

- [ ] **Step 6: Lint**

Run: `golangci-lint run ./core/service/websocket/client-msg-handler/...`
Expected: no findings.

- [ ] **Step 7: Commit**

```bash
git add core/service/websocket/client-msg-handler/
git commit -m "feat(metrics): record inbound client message count, outcome and latency"
```

---

## Task 8: Webhook callback metrics (Tier 2)

**Files:**
- Modify: `server/webhook/sender.go`
- Modify: `server/webhook/sync.go`
- Modify: `server/webhook/async.go`
- Create: `server/webhook/sender_observer_test.go`
- Create: `pkg/metrics/callback.go`
- Create: `pkg/metrics/callback_test.go`

**Interfaces:**
- Consumes: `ClassifyCallbackError` (Task 1); `meterName`, `mustCounter`, `mustHistogram` (Task 3).
- Produces:
  - In `webhook`: `type CallObserver interface { ObserveCall(eventType, mode string, dur time.Duration, statusCode int, err error); ObserveRetry(eventType, mode string); ObserveDropped(eventType string) }`
  - In `webhook`: `func (s *Sender) SetObserver(obs CallObserver)`, `const ModeSync = "sync"`, `const ModeAsync = "async"`
  - In `metrics`: `func NewCallbackMetrics() *CallbackMetrics` implementing `webhook.CallObserver` structurally
  - In `metrics`: `func (c *CallbackMetrics) RegisterBreakerGauge(src BreakerStateSource) error`, `type BreakerStateSource interface { OpenSince() (time.Time, bool) }`

**Background — the isolation rule.** `server/webhook` must NOT import
`pkg/metrics` (spec: Global Constraints). The interface is declared inside
`webhook`; `pkg/metrics` provides a type that satisfies it structurally, and
`cmd/pipewave-server/main.go` is the only place the two meet. Keep the observer
nil-able so existing `webhook` tests keep passing untouched.

`Sender.Post` (`sender.go:32`) has signature
`Post(ctx, eventType, callbackID string, data any, timeout time.Duration) (int, []byte, error)`
and is the single chokepoint every callback flows through — both `SyncCaller.Call`
(`sync.go:98`) and `AsyncDispatcher.deliver` (`async.go:108`).

`Post` cannot know whether it was called from the sync or async path, so `mode`
is carried on the `Sender`: `main.go` builds separate `Sender` instances for
sync and async already? **Verify first** — read `main.go` lines 55-70. It builds
ONE `sender` shared by both `async` and `syncCaller`. So `mode` must be passed
per call, not stored on the Sender. Use an explicit `PostWithMode` wrapper:
keep `Post` as-is (defaulting to sync) and have `AsyncDispatcher` call
`PostWithMode(..., ModeAsync)`.

- [ ] **Step 1: Write the failing test for the observer hook**

Create `server/webhook/sender_observer_test.go`:

```go
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
```

Confirm `webhook.EventOnCloseConnection` is the real constant name first:
`grep -n "EventOnCloseConnection\|Event.*=" server/webhook/envelope.go`.
Use whatever the package actually exports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/webhook/... -run TestSender_Observer -v`
Expected: FAIL — `sender.SetObserver` undefined.

- [ ] **Step 3: Add the observer seam to Sender**

In `server/webhook/sender.go`, add after the imports:

```go
// Call modes, used as the "mode" metric label.
const (
	ModeSync  = "sync"
	ModeAsync = "async"
)

// CallObserver receives callback outcomes for instrumentation.
//
// Declared here, in the webhook package, so that webhook never imports the
// metrics package: the concrete implementation lives in pkg/metrics and is
// wired in cmd/pipewave-server/main.go. A nil observer disables observation,
// which is the default.
type CallObserver interface {
	// ObserveCall reports one completed HTTP attempt. statusCode is 0 when the
	// request never got a response.
	ObserveCall(eventType, mode string, dur time.Duration, statusCode int, err error)
	// ObserveRetry reports that an attempt is being retried.
	ObserveRetry(eventType, mode string)
	// ObserveDropped reports a callback abandoned after exhausting retries.
	ObserveDropped(eventType string)
}
```

Add the field to `Sender` and a setter:

```go
// SetObserver attaches an observer. Safe to leave unset.
func (s *Sender) SetObserver(obs CallObserver) { s.obs = obs }
```

Add `obs CallObserver` to the `Sender` struct.

Rename the body of `Post` to `PostWithMode` and keep `Post` as a thin wrapper so
no existing caller changes:

```go
// Post delivers a signed callback using the sync mode label.
func (s *Sender) Post(ctx context.Context, eventType, callbackID string, data any, timeout time.Duration) (int, []byte, error) {
	return s.PostWithMode(ctx, eventType, callbackID, data, timeout, ModeSync)
}

// PostWithMode delivers a signed callback, tagging observations with mode.
func (s *Sender) PostWithMode(ctx context.Context, eventType, callbackID string, data any, timeout time.Duration, mode string) (status int, body []byte, err error) {
	start := time.Now()
	defer func() {
		if s.obs != nil {
			s.obs.ObserveCall(eventType, mode, time.Since(start), status, err)
		}
	}()

	// ... existing Post body unchanged, but ensure it assigns to the NAMED
	// return values (status, body, err) so the deferred observer sees them.
}
```

Read the current `Post` body carefully and convert its `return a, b, c`
statements to assign the named results. The named returns are what make the
single `defer` correct for every exit path.

- [ ] **Step 4: Run observer tests to verify they pass**

Run: `go test ./server/webhook/... -run TestSender -v`
Expected: PASS, including the pre-existing `TestSender_PostSignedEnvelope`.

- [ ] **Step 5: Count retries and drops**

In `server/webhook/sync.go`, inside `SyncCaller.Call`'s retry loop, before each
retry attempt after the first:

```go
		if attempt > 0 && c.sender.obs != nil {
			c.sender.obs.ObserveRetry(eventType, ModeSync)
		}
```

Adapt to the loop's actual variable names — read `sync.go:98` onward first.

In `server/webhook/async.go`, `deliver`:
- Change the `sender.Post(...)` call to `sender.PostWithMode(..., ModeAsync)`.
- Before each retry after the first, call `ObserveRetry(job.eventType, ModeAsync)`.
- When retries are exhausted and the job is abandoned, call
  `ObserveDropped(job.eventType)`.

Read the existing retry/drop logic and place the calls on the paths that already
log the retry and the drop.

- [ ] **Step 6: Write the failing test for CallbackMetrics**

Create `pkg/metrics/callback_test.go`:

```go
package metrics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestCallbackMetrics_Duration(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	c.ObserveCall("inspect_token", "sync", 150*time.Millisecond, 200, nil)

	got := findMetric(t, collect(t, reader), "pipewave_callback_duration_seconds")
	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Equal(t, uint64(1), hist.DataPoints[0].Count)
	require.InDelta(t, 0.15, hist.DataPoints[0].Sum, 0.001)
}

func TestCallbackMetrics_NoErrorMetricOnSuccess(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	c.ObserveCall("inspect_token", "sync", time.Millisecond, 200, nil)

	rm := collect(t, reader)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "pipewave_callback_errors_total" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			require.Empty(t, sum.DataPoints, "a 200 must not record an error")
		}
	}
}

func TestCallbackMetrics_ErrorReasons(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	c.ObserveCall("handle_message", "sync", time.Millisecond, 500, nil)
	c.ObserveCall("handle_message", "sync", time.Millisecond, 0, context.DeadlineExceeded)
	c.ObserveCall("handle_message", "async", time.Millisecond, 404, nil)

	got := findMetric(t, collect(t, reader), "pipewave_callback_errors_total")
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{
		"event_type": "handle_message", "mode": "sync", "reason": "status_5xx"}))
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{
		"event_type": "handle_message", "mode": "sync", "reason": "timeout"}))
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{
		"event_type": "handle_message", "mode": "async", "reason": "status_4xx"}))
}

func TestCallbackMetrics_RetryAndDropped(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	c.ObserveRetry("on_close_connection", "async")
	c.ObserveRetry("on_close_connection", "async")
	c.ObserveDropped("on_close_connection")

	rm := collect(t, reader)
	retries := findMetric(t, rm, "pipewave_callback_retries_total")
	require.Equal(t, int64(2), sumFor(t, retries, map[string]string{
		"event_type": "on_close_connection", "mode": "async"}))

	dropped := findMetric(t, rm, "pipewave_callback_dropped_total")
	require.Equal(t, int64(1), sumFor(t, dropped, map[string]string{
		"event_type": "on_close_connection"}))
}

type stubBreaker struct {
	open  bool
	since time.Time
}

func (s *stubBreaker) OpenSince() (time.Time, bool) { return s.since, s.open }

func TestCallbackMetrics_BreakerGauge(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	br := &stubBreaker{open: false}
	require.NoError(t, c.RegisterBreakerGauge(br))

	got := findMetric(t, collect(t, reader), "pipewave_callback_breaker_open")
	g, ok := got.Data.(metricdata.Gauge[int64])
	require.True(t, ok)
	require.Equal(t, int64(0), g.DataPoints[0].Value)

	br.open = true
	br.since = time.Now()
	got = findMetric(t, collect(t, reader), "pipewave_callback_breaker_open")
	g, _ = got.Data.(metricdata.Gauge[int64])
	require.Equal(t, int64(1), g.DataPoints[0].Value)
}

func TestCallbackMetrics_UnknownErrorIsOther(t *testing.T) {
	reader := newTestReader(t)
	c := metrics.NewCallbackMetrics()

	c.ObserveCall("ping", "sync", time.Millisecond, 0, errors.New("boom"))

	got := findMetric(t, collect(t, reader), "pipewave_callback_errors_total")
	require.Equal(t, int64(1), sumFor(t, got, map[string]string{
		"event_type": "ping", "mode": "sync", "reason": "other"}))
}
```

- [ ] **Step 7: Run test to verify it fails**

Run: `go test ./pkg/metrics/... -run TestCallbackMetrics -v`
Expected: FAIL — `metrics.NewCallbackMetrics` undefined.

- [ ] **Step 8: Write pkg/metrics/callback.go**

```go
package metrics

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// CallbackMetrics instruments outbound webhook callbacks.
//
// It structurally satisfies webhook.CallObserver. The dependency points this
// way on purpose: server/webhook declares the interface and never imports this
// package, so webhook stays free of metrics plumbing.
type CallbackMetrics struct {
	duration metric.Float64Histogram
	errors   metric.Int64Counter
	retries  metric.Int64Counter
	dropped  metric.Int64Counter
}

// NewCallbackMetrics builds the Tier 2 callback instruments from the global
// MeterProvider. Safe when no provider is installed (no-op instruments).
func NewCallbackMetrics() *CallbackMetrics {
	meter := otel.GetMeterProvider().Meter(meterName)
	return &CallbackMetrics{
		duration: mustHistogram(meter, "pipewave_callback_duration_seconds",
			"Latency of one outbound callback attempt"),
		errors: mustCounter(meter, "pipewave_callback_errors_total",
			"Total failed callback attempts, by reason"),
		retries: mustCounter(meter, "pipewave_callback_retries_total",
			"Total callback retry attempts"),
		dropped: mustCounter(meter, "pipewave_callback_dropped_total",
			"Total callbacks abandoned after exhausting retries"),
	}
}

// ObserveCall records one completed callback attempt. A successful attempt
// records only the duration; errors additionally increment errors_total with a
// bounded reason label.
func (c *CallbackMetrics) ObserveCall(eventType, mode string, dur time.Duration, statusCode int, err error) {
	c.duration.Record(context.Background(), dur.Seconds(), metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.String("mode", mode),
	))

	reason := ClassifyCallbackError(err, statusCode)
	if reason == "" {
		return
	}
	c.errors.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.String("mode", mode),
		attribute.String("reason", reason),
	))
}

// ObserveRetry counts one retry attempt.
func (c *CallbackMetrics) ObserveRetry(eventType, mode string) {
	c.retries.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.String("mode", mode),
	))
}

// ObserveDropped counts one callback abandoned after retries.
func (c *CallbackMetrics) ObserveDropped(eventType string) {
	c.dropped.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("event_type", eventType),
	))
}

// BreakerStateSource reports circuit-breaker state.
// webhook.CircuitBreaker satisfies it via OpenSince.
type BreakerStateSource interface {
	OpenSince() (time.Time, bool)
}

// RegisterBreakerGauge publishes pipewave_callback_breaker_open as 0 or 1,
// read live on each scrape from the existing breaker state — no new state.
func (c *CallbackMetrics) RegisterBreakerGauge(src BreakerStateSource) error {
	meter := otel.GetMeterProvider().Meter(meterName)
	g, err := meter.Int64ObservableGauge("pipewave_callback_breaker_open",
		metric.WithDescription("1 when the callback circuit breaker is open, else 0"))
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		var v int64
		if _, open := src.OpenSince(); open {
			v = 1
		}
		o.ObserveInt64(g, v)
		return nil
	}, g)
	if err != nil {
		slog.Warn("metrics: register breaker gauge failed", slog.Any("error", err))
	}
	return err
}
```

`ObserveCall` uses `context.Background()` because the observer is called from a
`defer` where the request context may already be cancelled — a cancelled context
would drop the recording.

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./pkg/metrics/... ./server/webhook/... -v`
Expected: PASS, including all pre-existing webhook tests unmodified.

- [ ] **Step 10: Lint**

Run: `golangci-lint run ./pkg/metrics/... ./server/webhook/...`
Expected: no findings.

- [ ] **Step 11: Commit**

```bash
git add pkg/metrics/callback.go pkg/metrics/callback_test.go server/webhook/
git commit -m "feat(metrics): webhook callback duration, errors, retries and breaker gauge

Observation enters at the single Sender.Post chokepoint via a nil-able
CallObserver declared in the webhook package, so webhook keeps no metrics
imports and its existing tests are untouched."
```

---

## Task 9: Container wiring and docs

**Files:**
- Modify: `cmd/pipewave-server/main.go`
- Modify: `server-config.example.yaml`
- Modify: `README.md`

**Interfaces:**
- Consumes: everything from Tasks 3-8.
- Produces: nothing.

**Background:** `main.go` starts listeners with the `serve(name, srv)` helper,
which calls `fatal` on error. Do NOT reuse it for metrics — a metrics bind
failure must not kill the server. Shutdown is the block after `<-rootCtx.Done()`.

**Design:** the metrics provider lives inside the DI graph (registered in Task 4),
but `main.go` holds only `ModuleDelivery`. Rather than let `main.go` build a
second provider — two providers would fight over the global MeterProvider —
`NewDI` stays the single owner and `ModuleDelivery` grows three methods.

The provider owns its own listener (`Provider.ListenAndServe`, Task 4) because
the port lives in `METRICS.PORT`, which is pipewave config that `main.go` does not
read. Exposing `ServeMetrics()` rather than an `http.Handler` keeps the port in
exactly one place.

`CallbackObserver` returns `any` so `core/delivery` need not import
`server/webhook`; `main.go` type-asserts it.

- [ ] **Step 1: Extend the ModuleDelivery interface**

In `core/delivery/module.go`, add to the `ModuleDelivery` interface (the file
already imports `context` and `net/http`):

```go
	// ServeMetrics starts the metrics listener and blocks. Returns nil
	// immediately when metrics are disabled. Callers should log, not exit,
	// on error — a metrics listener must never take the server down.
	ServeMetrics() error
	// ShutdownMetrics stops the metrics listener.
	ShutdownMetrics(ctx context.Context) error
	// CallbackObserver returns the webhook call observer, or nil when metrics
	// are disabled. Typed as any so core/delivery does not import
	// server/webhook; the container type-asserts it to webhook.CallObserver.
	CallbackObserver() any
```

- [ ] **Step 2: Implement them on moduleDelivery**

In `core/delivery/module/0.0.new.go`:

- Add these imports: `log/slog`, `"github.com/pipewave-dev/go-pkg/pkg/metrics"`,
  `metricsprovider "github.com/pipewave-dev/go-pkg/provider/metrics-provider"`.
- Add to the `moduleDelivery` struct:

```go
	metricsProvider *metricsprovider.Provider
	callbackMetrics *metrics.CallbackMetrics
```

- Add to the `NewDI` struct literal:

```go
		metricsProvider: do.MustInvoke[*metricsprovider.Provider](i),
```

- After `ins.registerHandlers()`, register the gauges and build the callback
  metrics only when metrics are actually enabled:

```go
	ins.registerHandlers()

	// Gauges read live connection state on each scrape, so they cannot drift.
	if err := ins.metricsProvider.Metrics().RegisterConnectionGauges(ins.monitoringSvc); err != nil {
		slog.Warn("metrics: register connection gauges failed", slog.Any("error", err))
	}
	// Handler() is nil exactly when metrics are disabled; skip building the
	// callback observer so CallbackObserver() reports nil to the container.
	if ins.metricsProvider.Handler() != nil {
		ins.callbackMetrics = metrics.NewCallbackMetrics()
	}
```

`ins.monitoringSvc` is already a field on `moduleDelivery` and satisfies
`metrics.ConnectionStatsSource`.

Create `core/delivery/module/4.metrics.go`:

```go
package moduledelivery

import "context"

// ServeMetrics starts the metrics listener; no-op when metrics are disabled.
func (m *moduleDelivery) ServeMetrics() error {
	return m.metricsProvider.ListenAndServe()
}

// ShutdownMetrics stops the metrics listener.
func (m *moduleDelivery) ShutdownMetrics(ctx context.Context) error {
	return m.metricsProvider.Shutdown(ctx)
}

// CallbackObserver returns the webhook call observer, or nil when metrics are
// disabled. Typed as any so core/delivery does not import server/webhook.
func (m *moduleDelivery) CallbackObserver() any {
	if m.callbackMetrics == nil {
		return nil
	}
	return m.callbackMetrics
}
```

Returning a typed nil through an `any` would produce a non-nil interface, so the
explicit nil check above matters — without it `main.go`'s `!= nil` guard would
pass and it would call methods on a nil `*CallbackMetrics`.

- [ ] **Step 3: Build to confirm the interface is satisfied**

Run: `go build ./...`
Expected: builds. If `moduleDelivery` does not satisfy `ModuleDelivery`, the
compiler names the missing method.

- [ ] **Step 4: Attach the callback observer in main.go**

In `cmd/pipewave-server/main.go`, add the import
`"github.com/pipewave-dev/go-pkg/pkg/metrics"`.

After `sender := webhook.NewSender(...)` (line 55):

```go
	sender := webhook.NewSender(srvCfg.Callbacks.BaseURL, signer)
	if obs, ok := pw.CallbackObserver().(webhook.CallObserver); ok {
		sender.SetObserver(obs)
	}
```

After `breaker := webhook.NewCircuitBreaker(...)` (line 66):

```go
	breaker := webhook.NewCircuitBreaker(srvCfg.Callbacks.Breaker.Threshold, srvCfg.Callbacks.Breaker.Cooldown)
	if cm, ok := pw.CallbackObserver().(*metrics.CallbackMetrics); ok {
		if err := cm.RegisterBreakerGauge(breaker); err != nil {
			slog.Warn("metrics: register breaker gauge failed", "error", err)
		}
	}
```

Inside the `if srvCfg.Callbacks.Ping.Enabled` block, after
`pingSender := webhook.NewSender(pingURL, signer)` (~line 85), so ping failures
are visible too:

```go
		pingSender := webhook.NewSender(pingURL, signer)
		if obs, ok := pw.CallbackObserver().(webhook.CallObserver); ok {
			pingSender.SetObserver(obs)
		}
```

The type assertions need no extra `!= nil`: `CallbackObserver()` returns an
untyped nil when metrics are off, so `ok` is false.

- [ ] **Step 5: Start and stop the metrics listener**

After `go serve("admin", adminSrv)` (~line 130):

```go
	go func() {
		// Log, never fatal: a metrics listener that cannot bind must not take
		// the server down.
		if err := pw.ServeMetrics(); err != nil {
			slog.Error("[pipewave-server] metrics listener stopped", "error", err)
		}
	}()
```

In the shutdown block, before `pw.Shutdown()`:

```go
	_ = pw.ShutdownMetrics(shutdownCtx)
```

`Provider.Shutdown` is also registered as a DI cleanup task in Task 4, but
`http.Server.Shutdown` and `MeterProvider.Shutdown` are both idempotent, so the
double call is harmless.

- [ ] **Step 6: Build and run the full suite**

Run: `go build ./... && go test ./... 2>&1 | tail -40`
Expected: PASS. Note some repo tests use testcontainers and need Docker; if
those were already failing before this change, they are out of scope.

- [ ] **Step 7: Document the config**

Append to `server-config.example.yaml`, as a top-level block (it is pipewave
config, not `SERVER`):

```yaml
# Prometheus metrics. Served on a dedicated listener so Prometheus can scrape
# without being granted admin API access.
METRICS:
  ENABLED: false
  PORT: 9090
  PATH: "/metrics"
  # App-level msg_type values that get their own metric label. Anything not
  # listed is reported as "other" — msg_type is client-controlled, so this
  # list is what bounds label cardinality. Max 32 printable ASCII chars each.
  MSG_TYPE_ALLOWLIST: []
```

- [ ] **Step 8: Document the endpoint in README**

Add a `### Metrics` section after the `### Config` section:

```markdown
### Metrics

Set `METRICS.ENABLED: true` to expose Prometheus metrics on a dedicated
listener (`METRICS.PORT`, default `9090`, path `METRICS.PATH`, default
`/metrics`). The listener is separate from the client (`:8080`) and admin
(`:8081`) listeners and requires no API key, so Prometheus can scrape it
without admin credentials — keep the port unexposed to the internet.

```bash
curl -s localhost:9090/metrics | grep pipewave_
```

Exported metrics: connection lifecycle
(`pipewave_connections_active`, `pipewave_users_active`,
`pipewave_connections_accepted_total`, `pipewave_connections_rejected_total`,
`pipewave_connection_duration_seconds`), inbound client messages
(`pipewave_client_messages_total`,
`pipewave_client_message_duration_seconds`), outbound callbacks
(`pipewave_callback_duration_seconds`, `pipewave_callback_errors_total`,
`pipewave_callback_retries_total`, `pipewave_callback_dropped_total`,
`pipewave_callback_breaker_open`), and `pipewave_build_info`.

`msg_type` is client-controlled, so only values listed in
`METRICS.MSG_TYPE_ALLOWLIST` get their own label; everything else is
reported as `other`.

Go embedders: `pipewave` never installs a global OTEL `MeterProvider`. Set
your own before calling `pipewave.New()` and pipewave's instruments flow into
your registry; set none and every metric call is a no-op.
```

- [ ] **Step 9: Manual smoke test**

```bash
docker compose up -d valkey postgres
go run ./examples/rest-backend -addr :9000 &
# enable metrics in a local override config first
go run ./cmd/pipewave-server -config server-config.example.yaml &
sleep 3
curl -s localhost:9090/metrics | grep -c pipewave_   # expect > 0
curl -s localhost:9090/metrics | grep pipewave_build_info
```

Then drive one connection rejection and confirm the counter moves:

```bash
curl -s "localhost:8080/gw"                          # no ?tk= -> missing_token
curl -s localhost:9090/metrics | grep connections_rejected_total
```

Expected: a series with `reason="missing_token"` and value `1`.

- [ ] **Step 10: Commit**

```bash
git add cmd/pipewave-server/main.go core/delivery/ server-config.example.yaml README.md
git commit -m "feat(metrics): wire metrics listener, gauges and callback observer in container"
```

---

## Verification Checklist

Run before declaring the feature done:

- [ ] `go build ./...` succeeds.
- [ ] `go test ./pkg/metrics/... ./export/types/... ./provider/metrics-provider/... ./server/webhook/... -v` passes.
- [ ] `golangci-lint run ./...` reports no new findings.
- [ ] With `METRICS.ENABLED: false` (the default), `:9090` is NOT listening and no behaviour changes.
- [ ] With `METRICS.ENABLED: true`, `curl localhost:9090/metrics` lists every metric named in the README.
- [ ] `grep -rn "pkg/metrics" server/webhook/` returns nothing (isolation constraint).
- [ ] `grep -rn "SetMeterProvider" pkg/metrics/` returns nothing (library must not install a provider).
- [ ] No label named `user_id`, `session_id`, or `instance_id` exists: `curl -s localhost:9090/metrics | grep -E 'user_id=|session_id=|instance_id='` returns nothing.
- [ ] `container_id` appears only on `pipewave_build_info`.
- [ ] Pre-existing `server/webhook` tests pass without modification.
