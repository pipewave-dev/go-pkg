# Callback Failure Resilience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cho pipewave-server tự phản ứng khi backend callback endpoint fail — sync retry, health-ping chủ động, và shutdown-on-unhealthy — với mọi hành vi mới opt-in qua config.

**Architecture:** Một `HealthMonitor` hội tụ tín hiệu từ circuit breaker (traffic thật) và pinger (chủ động). Khi backend unhealthy quá ngưỡng, monitor gọi một callback `onUnhealthy` do `main.go` cung cấp — package `webhook` không biết gì về HTTP server hay `os.Exit`. Shutdown mode tái dùng graceful sequence hiện có bằng cách cancel `rootCtx`.

**Tech Stack:** Go (stdlib `net/http`, `context`, `sync/atomic`, `log/slog`), koanf config (`pkg/koanf`), testify (`require`), `httptest` cho test.

## Global Constraints

- Module path: `github.com/pipewave-dev/go-pkg`.
- Test package convention: external `_test` package (`package webhook_test`, `package serverconfig_test`); assert với `github.com/stretchr/testify/require`; server callback tests dùng `net/http/httptest`.
- **Backward compatibility là bắt buộc:** mọi field config mới có default = hành vi hiện tại. Config file cũ phải chạy y hệt.
- Config sống trong section `SERVER.CALLBACKS`; koanf tag UPPER_SNAKE; env prefix `APP`.
- Callback envelope đã signed; event constant sống trong `server/webhook/envelope.go`.
- `git commit` message kết thúc bằng: `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`
- Chạy toàn bộ test package trước mỗi commit: `go test ./server/... ./cmd/...`

---

## File Structure

| File | Trách nhiệm | Create/Modify |
|------|-------------|---------------|
| `server/config/config.go` | Struct + defaults + validation cho field mới | Modify |
| `server/config/config_test.go` | Test defaults + validation mới | Modify |
| `server/webhook/envelope.go` | Thêm `EventPing` | Modify |
| `server/webhook/sync.go` | `SyncCaller` retry + `CircuitBreaker.OpenSince` | Modify |
| `server/webhook/sync_test.go` | Test retry + fix call-site chữ ký mới | Modify |
| `server/webhook/ping.go` | `Pinger` (mới) | Create |
| `server/webhook/ping_test.go` | Test Pinger | Create |
| `server/webhook/health.go` | `HealthMonitor` (mới) | Create |
| `server/webhook/health_test.go` | Test HealthMonitor | Create |
| `cmd/pipewave-server/main.go` | Wiring: đọc config mới, pinger, monitor, breaker-watcher, onUnhealthy | Modify |
| `server/restapi/mux.go` | `/healthz` phản ánh monitor | Modify |
| `server/restapi/restapi_test.go` | Test healthz phản ánh monitor | Modify |
| `server/README.md` | Tài liệu hành vi + config mới | Modify |
| `server-config.example.yaml` | Ví dụ config mới | Modify |

**Thứ tự task theo dependency:** config → breaker method + sync retry → ping → health → wiring → docs.

---

## Task 1: Config — struct, defaults, validation

**Files:**
- Modify: `server/config/config.go`
- Test: `server/config/config_test.go`

**Interfaces:**
- Consumes: nothing (leaf).
- Produces: `serverconfig.CallbacksT` mở rộng với `AsyncBackoff []time.Duration`, `SyncRetry SyncRetryT{Max int, Backoff time.Duration}`, `Breaker BreakerT{Threshold int, Cooldown time.Duration}`, `Ping PingT{Enabled bool, Path string, Interval, Timeout time.Duration, BootCheck bool, FailThreshold int}`, `UnhealthyAction string`, `BreakerOpenShutdown time.Duration`. Constants `UnhealthyActionShutdown = "shutdown"`, `UnhealthyActionLogOnly = "log-only"`.

- [ ] **Step 1: Write the failing test**

Thêm vào `server/config/config_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/config/ -run 'CallbackResilience|PingDefaults|BadUnhealthy' -v`
Expected: FAIL (fields/constants không tồn tại — compile error).

- [ ] **Step 3: Add struct fields and constants**

Trong `server/config/config.go`, thêm constants vào block const hiện có:

```go
	UnhealthyActionShutdown = "shutdown"
	UnhealthyActionLogOnly  = "log-only"
```

Mở rộng `CallbacksT`:

```go
type CallbacksT struct {
	BaseURL       string        `koanf:"BASE_URL"`
	Signature     SignatureT    `koanf:"SIGNATURE"`
	HandleMessage HandleMsgT    `koanf:"HANDLE_MESSAGE"`
	SyncTimeout   time.Duration `koanf:"SYNC_TIMEOUT"`
	AsyncRetryMax int           `koanf:"ASYNC_RETRY_MAX"`

	AsyncBackoff        []time.Duration `koanf:"ASYNC_BACKOFF"`
	SyncRetry           SyncRetryT      `koanf:"SYNC_RETRY"`
	Breaker             BreakerT        `koanf:"BREAKER"`
	Ping                PingT           `koanf:"PING"`
	UnhealthyAction     string          `koanf:"UNHEALTHY_ACTION"`
	BreakerOpenShutdown time.Duration   `koanf:"BREAKER_OPEN_SHUTDOWN"`
}

type SyncRetryT struct {
	Max     int           `koanf:"MAX"`
	Backoff time.Duration `koanf:"BACKOFF"`
}

type BreakerT struct {
	Threshold int           `koanf:"THRESHOLD"`
	Cooldown  time.Duration `koanf:"COOLDOWN"`
}

type PingT struct {
	Enabled       bool          `koanf:"ENABLED"`
	Path          string        `koanf:"PATH"`
	Interval      time.Duration `koanf:"INTERVAL"`
	Timeout       time.Duration `koanf:"TIMEOUT"`
	BootCheck     bool          `koanf:"BOOT_CHECK"`
	FailThreshold int           `koanf:"FAIL_THRESHOLD"`
}
```

- [ ] **Step 4: Add defaults**

Trong `loadDefault()`, thêm (cuối hàm).

**Quyết định về `BootCheck`:** koanf không phân biệt được "unset" với "false" cho một bool zero-value, nên `BOOT_CHECK` KHÔNG được expose làm knob cấu hình. Thay vào đó `loadDefault` luôn set `BootCheck = true` khi `Ping.Enabled == true` — tức "bật ping thì boot-check luôn chạy". Field vẫn giữ trong struct để tầng wiring (Task 6) đọc.

```go
	if c.Callbacks.SyncRetry.Max <= 0 {
		c.Callbacks.SyncRetry.Max = 1
	}
	if c.Callbacks.SyncRetry.Backoff <= 0 {
		c.Callbacks.SyncRetry.Backoff = 100 * time.Millisecond
	}
	if c.Callbacks.Breaker.Threshold <= 0 {
		c.Callbacks.Breaker.Threshold = 5
	}
	if c.Callbacks.Breaker.Cooldown <= 0 {
		c.Callbacks.Breaker.Cooldown = 10 * time.Second
	}
	if c.Callbacks.Ping.Enabled {
		if c.Callbacks.Ping.Path == "" {
			c.Callbacks.Ping.Path = "/pipewave/ping"
		}
		if c.Callbacks.Ping.Interval <= 0 {
			c.Callbacks.Ping.Interval = 30 * time.Second
		}
		if c.Callbacks.Ping.Timeout <= 0 {
			c.Callbacks.Ping.Timeout = 3 * time.Second
		}
		if c.Callbacks.Ping.FailThreshold <= 0 {
			c.Callbacks.Ping.FailThreshold = 3
		}
		c.Callbacks.Ping.BootCheck = true // luôn boot-check khi ping enabled
	}
	if c.Callbacks.UnhealthyAction == "" {
		c.Callbacks.UnhealthyAction = UnhealthyActionLogOnly
	}
```

> **Ghi chú simplification:** field `BootCheck` giữ trong struct (đọc bởi wiring) nhưng KHÔNG expose làm knob cấu hình — luôn `true` khi ping enabled. Test `TestLoad_PingDefaultsWhenEnabled` assert `BootCheck == true` phản ánh điều này.

- [ ] **Step 5: Add validation**

Trong `validate()`, thêm trước `return nil`:

```go
	switch c.Callbacks.UnhealthyAction {
	case UnhealthyActionShutdown, UnhealthyActionLogOnly:
	default:
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.UNHEALTHY_ACTION must be %q or %q, got %q",
			UnhealthyActionShutdown, UnhealthyActionLogOnly, c.Callbacks.UnhealthyAction)
	}
	if c.Callbacks.SyncRetry.Max < 1 {
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.SYNC_RETRY.MAX must be >= 1, got %d", c.Callbacks.SyncRetry.Max)
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./server/config/ -v`
Expected: PASS (bao gồm cả test cũ — defaults không phá vỡ config hiện tại).

- [ ] **Step 7: Commit**

```bash
git add server/config/config.go server/config/config_test.go
git commit -m "feat(config): add callback resilience knobs (sync-retry, breaker, ping, unhealthy-action)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: CircuitBreaker.OpenSince + SyncCaller retry

**Files:**
- Modify: `server/webhook/sync.go`
- Test: `server/webhook/sync_test.go`

**Interfaces:**
- Consumes: nothing new (config đọc ở wiring, không ở đây).
- Produces:
  - `func (b *CircuitBreaker) OpenSince() (time.Time, bool)` — trả `(openedAt, true)` nếu breaker hiện đang open (tức `failures >= threshold` và chưa qua cooldown-probe-success), ngược lại `(time.Time{}, false)`.
  - Chữ ký MỚI: `func NewSyncCaller(sender *Sender, breaker *CircuitBreaker, retryMax int, backoff time.Duration) *SyncCaller`. **Breaking change** — mọi call-site phải cập nhật (test task này + main.go task 6).

- [ ] **Step 1: Write failing tests**

Sửa `server/webhook/sync_test.go`. Trước tiên **cập nhật 4 call-site `NewSyncCaller` hiện có** thêm `, 1, 0` (no-retry) để giữ hành vi cũ:

```go
// TestSyncCaller_DecodesResponse:
c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)), webhook.NewCircuitBreaker(5, 10*time.Second), 1, 0)
// TestSyncCaller_Non2xxReturnsCallError_NoBreakerTripOn4xx:
c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)), webhook.NewCircuitBreaker(2, 10*time.Second), 1, 0)
// TestSyncCaller_BreakerOpensOn5xxAndRecovers:
c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)), breaker, 1, 0)
// TestSyncCaller_TransportErrorCountsAsFailure:
c := webhook.NewSyncCaller(webhook.NewSender("http://127.0.0.1:1", newTestSigner(t)), webhook.NewCircuitBreaker(1, time.Minute), 1, 0)
```

Thêm test mới:

```go
func TestSyncCaller_Retries5xxThenSucceeds(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)),
		webhook.NewCircuitBreaker(10, time.Minute), 3, time.Millisecond)
	require.NoError(t, c.Call(context.Background(), webhook.EventInspectToken, nil, time.Second, nil))
	require.Equal(t, int64(2), hits.Load()) // 1 fail + 1 success
}

func TestSyncCaller_Does_Not_Retry_4xx(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()

	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)),
		webhook.NewCircuitBreaker(10, time.Minute), 3, time.Millisecond)
	var ce *webhook.CallError
	require.ErrorAs(t, c.Call(context.Background(), webhook.EventOnNewConnection, nil, time.Second, nil), &ce)
	require.Equal(t, int64(1), hits.Load()) // 4xx = câu trả lời chủ ý, không retry
}

func TestSyncCaller_ReusesCallbackIDAcrossRetries(t *testing.T) {
	var ids sync.Map
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Meta struct {
				CallbackID string `json:"id"`
			} `json:"meta"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		ids.Store(body.Meta.CallbackID, true)
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := webhook.NewSyncCaller(webhook.NewSender(srv.URL, newTestSigner(t)),
		webhook.NewCircuitBreaker(10, time.Minute), 3, time.Millisecond)
	require.NoError(t, c.Call(context.Background(), webhook.EventInspectToken, nil, time.Second, nil))
	count := 0
	ids.Range(func(_, _ any) bool { count++; return true })
	require.Equal(t, 1, count) // cùng callbackID cho cả 2 attempt
}

func TestCircuitBreaker_OpenSince(t *testing.T) {
	b := webhook.NewCircuitBreaker(2, time.Minute)
	_, ok := b.OpenSince()
	require.False(t, ok)
	b.Record(false)
	b.Record(false)
	at, ok := b.OpenSince()
	require.True(t, ok)
	require.False(t, at.IsZero())
	b.Record(true) // success closes
	_, ok = b.OpenSince()
	require.False(t, ok)
}
```

Thêm import `encoding/json` và `sync` vào test file nếu chưa có.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/webhook/ -run 'Retries5xx|Does_Not_Retry|ReusesCallbackID|OpenSince' -v`
Expected: FAIL (chữ ký `NewSyncCaller` cũ, `OpenSince` chưa có).

- [ ] **Step 3: Add OpenSince to CircuitBreaker**

Trong `server/webhook/sync.go`, thêm method:

```go
// OpenSince trả thời điểm breaker chuyển open nếu HIỆN ĐANG open (chưa được
// một probe thành công đóng lại), cùng ok=true. Không mở → (time.Time{}, false).
func (b *CircuitBreaker) OpenSince() (time.Time, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures < b.threshold {
		return time.Time{}, false
	}
	return b.openedAt, true
}
```

- [ ] **Step 4: Rewrite SyncCaller with retry**

Thay `SyncCaller` struct, constructor, và `Call`:

```go
type SyncCaller struct {
	sender   *Sender
	breaker  *CircuitBreaker
	retryMax int
	backoff  time.Duration
}

func NewSyncCaller(sender *Sender, breaker *CircuitBreaker, retryMax int, backoff time.Duration) *SyncCaller {
	if retryMax < 1 {
		retryMax = 1
	}
	return &SyncCaller{sender: sender, breaker: breaker, retryMax: retryMax, backoff: backoff}
}

// Call posts the event and decodes a 2xx JSON response into out (out may be
// nil). Non-2xx returns *CallError. Only transport errors and 5xx are recorded
// as breaker failures AND retried (up to retryMax attempts, reusing one
// callbackID so receivers dedupe). 4xx and circuit-open short-circuit without
// retry. Retries stop early if ctx is done.
func (c *SyncCaller) Call(ctx context.Context, eventType string, data any, timeout time.Duration, out any) error {
	callbackID := NewCallbackID()
	var lastErr error
	for attempt := 0; attempt < c.retryMax; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(c.backoff):
			}
		}
		if !c.breaker.Allow() {
			return ErrCircuitOpen
		}
		status, body, err := c.sender.Post(ctx, eventType, callbackID, data, timeout)
		if err != nil {
			c.breaker.Record(false)
			lastErr = err
			continue // transport error → retry
		}
		if status < 200 || status >= 300 {
			c.breaker.Record(status < 500)
			if status < 500 {
				return &CallError{Status: status, Body: body} // 4xx: deliberate, no retry
			}
			lastErr = &CallError{Status: status, Body: body}
			continue // 5xx → retry
		}
		c.breaker.Record(true)
		if out != nil && len(body) > 0 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("webhook: decode %s response: %w", eventType, err)
			}
		}
		return nil
	}
	return lastErr
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./server/webhook/ -v`
Expected: PASS (test cũ với `,1,0` + test retry mới).

- [ ] **Step 6: Commit**

```bash
git add server/webhook/sync.go server/webhook/sync_test.go
git commit -m "feat(webhook): sync-caller bounded retry for transport/5xx + breaker OpenSince

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Pinger

**Files:**
- Modify: `server/webhook/envelope.go`
- Create: `server/webhook/ping.go`
- Test: `server/webhook/ping_test.go`

**Interfaces:**
- Consumes: `*Sender` (existing), `NewCallbackID` (existing).
- Produces:
  - Const `EventPing = "ping"` trong `envelope.go`.
  - `func NewPinger(sender *Sender, timeout time.Duration, threshold int) *Pinger`
  - `func (p *Pinger) Ping(ctx context.Context) error` — nil nếu 2xx.
  - `func (p *Pinger) Run(ctx context.Context, interval time.Duration, onHealthy, onUnhealthy func())` — block tới khi ctx done; fail streak >= threshold gọi `onUnhealthy` mỗi lần vượt ngưỡng, 2xx gọi `onHealthy` + reset streak.

- [ ] **Step 1: Write failing tests**

Tạo `server/webhook/ping_test.go`:

```go
package webhook_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

func TestPinger_Ping_2xxIsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	p := webhook.NewPinger(webhook.NewSender(srv.URL, newTestSigner(t)), time.Second, 3)
	require.NoError(t, p.Ping(context.Background()))
}

func TestPinger_Ping_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	p := webhook.NewPinger(webhook.NewSender(srv.URL, newTestSigner(t)), time.Second, 3)
	require.Error(t, p.Ping(context.Background()))
}

func TestPinger_Run_FiresUnhealthyAfterThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	var unhealthy atomic.Int64
	p := webhook.NewPinger(webhook.NewSender(srv.URL, newTestSigner(t)), time.Second, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx, time.Millisecond, func() {}, func() { unhealthy.Add(1); cancel() })

	require.Eventually(t, func() bool { return unhealthy.Load() >= 1 }, time.Second, time.Millisecond)
}

func TestPinger_Run_HealthyResetsStreak(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var healthy atomic.Int64
	p := webhook.NewPinger(webhook.NewSender(srv.URL, newTestSigner(t)), time.Second, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx, time.Millisecond, func() { healthy.Add(1) }, func() {})

	time.Sleep(10 * time.Millisecond)
	fail.Store(false)
	require.Eventually(t, func() bool { return healthy.Load() >= 1 }, time.Second, time.Millisecond)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/webhook/ -run 'Pinger' -v`
Expected: FAIL (`NewPinger`, `Ping`, `Run` chưa có; `EventPing` chưa có nếu dùng).

- [ ] **Step 3: Add EventPing constant**

Trong `server/webhook/envelope.go`, thêm vào block const event:

```go
	EventPing = "ping"
```

- [ ] **Step 4: Implement Pinger**

Tạo `server/webhook/ping.go`:

```go
package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Pinger chủ động POST một `ping` event tới backend callback endpoint để kiểm
// tra endpoint còn sống. Dùng cho boot-check (Ping) và runtime health (Run).
type Pinger struct {
	sender    *Sender
	timeout   time.Duration
	threshold int
}

func NewPinger(sender *Sender, timeout time.Duration, threshold int) *Pinger {
	if threshold < 1 {
		threshold = 1
	}
	return &Pinger{sender: sender, timeout: timeout, threshold: threshold}
}

// Ping gửi một ping và trả nil nếu backend trả 2xx.
func (p *Pinger) Ping(ctx context.Context) error {
	status, _, err := p.sender.Post(ctx, EventPing, NewCallbackID(), struct{}{}, p.timeout)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("webhook: ping returned status %d", status)
	}
	return nil
}

// Run ping theo interval tới khi ctx done. Chuỗi fail liên tiếp đạt threshold
// gọi onUnhealthy (mỗi lần đạt/vượt ngưỡng); một 2xx reset streak và gọi
// onHealthy.
func (p *Pinger) Run(ctx context.Context, interval time.Duration, onHealthy, onUnhealthy func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	streak := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.Ping(ctx); err != nil {
				streak++
				slog.Warn("[webhook] ping failed", "streak", streak, "error", err)
				if streak >= p.threshold {
					onUnhealthy()
				}
			} else {
				if streak > 0 {
					slog.Info("[webhook] ping recovered", "prev_streak", streak)
				}
				streak = 0
				onHealthy()
			}
		}
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./server/webhook/ -run 'Pinger' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/webhook/envelope.go server/webhook/ping.go server/webhook/ping_test.go
git commit -m "feat(webhook): active callback Pinger (boot-check + runtime ticker)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: HealthMonitor + breaker-open watcher

**Files:**
- Create: `server/webhook/health.go`
- Test: `server/webhook/health_test.go`

**Interfaces:**
- Consumes: `*CircuitBreaker` + `OpenSince()` (Task 2).
- Produces:
  - `func NewHealthMonitor(onUnhealthy func()) *HealthMonitor` — khởi tạo `healthy=true`.
  - `func (m *HealthMonitor) SetHealthy()`
  - `func (m *HealthMonitor) SetUnhealthy(reason string)` — log CRITICAL; gọi `onUnhealthy` đúng 1 lần (guard).
  - `func (m *HealthMonitor) IsHealthy() bool`
  - `func WatchBreakerOpen(ctx context.Context, b *CircuitBreaker, maxOpen time.Duration, m *HealthMonitor)` — ticker; nếu breaker open liên tục >= maxOpen → `m.SetUnhealthy(...)`. Block tới ctx done.

- [ ] **Step 1: Write failing tests**

Tạo `server/webhook/health_test.go`:

```go
package webhook_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

func TestHealthMonitor_FiresOnceOnUnhealthy(t *testing.T) {
	var fired atomic.Int64
	m := webhook.NewHealthMonitor(func() { fired.Add(1) })
	require.True(t, m.IsHealthy())
	m.SetUnhealthy("boom")
	m.SetUnhealthy("boom again")
	require.False(t, m.IsHealthy())
	require.Equal(t, int64(1), fired.Load())
}

func TestHealthMonitor_SetHealthyClearsFlagButNotRefire(t *testing.T) {
	var fired atomic.Int64
	m := webhook.NewHealthMonitor(func() { fired.Add(1) })
	m.SetHealthy() // no-op when already healthy
	require.True(t, m.IsHealthy())
	require.Equal(t, int64(0), fired.Load())
}

func TestWatchBreakerOpen_FiresWhenOpenTooLong(t *testing.T) {
	b := webhook.NewCircuitBreaker(1, time.Hour) // long cooldown → stays open
	b.Record(false)                              // open now
	var fired atomic.Int64
	m := webhook.NewHealthMonitor(func() { fired.Add(1) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go webhook.WatchBreakerOpen(ctx, b, 5*time.Millisecond, m)
	require.Eventually(t, func() bool { return fired.Load() >= 1 }, time.Second, time.Millisecond)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/webhook/ -run 'HealthMonitor|WatchBreakerOpen' -v`
Expected: FAIL (symbols chưa có).

- [ ] **Step 3: Implement HealthMonitor + watcher**

Tạo `server/webhook/health.go`:

```go
package webhook

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// HealthMonitor hội tụ tín hiệu sức khỏe backend từ nhiều nguồn (pinger,
// breaker watcher). onUnhealthy được gọi ĐÚNG MỘT LẦN ở lần chuyển
// healthy→unhealthy đầu tiên. Package này không biết callback đó làm gì —
// wiring ở main.go quyết định (graceful shutdown hoặc log-only).
type HealthMonitor struct {
	mu          sync.Mutex
	healthy     bool
	fired       bool
	onUnhealthy func()
}

func NewHealthMonitor(onUnhealthy func()) *HealthMonitor {
	return &HealthMonitor{healthy: true, onUnhealthy: onUnhealthy}
}

func (m *HealthMonitor) SetHealthy() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = true
}

func (m *HealthMonitor) SetUnhealthy(reason string) {
	m.mu.Lock()
	fire := !m.fired
	m.fired = true
	m.healthy = false
	m.mu.Unlock()
	if fire {
		slog.Error("[webhook] backend marked unhealthy", "reason", reason)
		if m.onUnhealthy != nil {
			m.onUnhealthy()
		}
	}
}

func (m *HealthMonitor) IsHealthy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.healthy
}

// WatchBreakerOpen báo unhealthy nếu breaker mở liên tục >= maxOpen. Ticker
// chạy ở min(maxOpen, 5s) để phát hiện kịp thời. Block tới ctx done.
func WatchBreakerOpen(ctx context.Context, b *CircuitBreaker, maxOpen time.Duration, m *HealthMonitor) {
	tick := maxOpen
	if tick > 5*time.Second || tick <= 0 {
		tick = 5 * time.Second
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if at, ok := b.OpenSince(); ok && time.Since(at) >= maxOpen {
				m.SetUnhealthy("circuit breaker open >= " + maxOpen.String())
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/webhook/ -run 'HealthMonitor|WatchBreakerOpen' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/webhook/health.go server/webhook/health_test.go
git commit -m "feat(webhook): HealthMonitor + breaker-open watcher

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: healthz reflects HealthMonitor

**Files:**
- Modify: `server/restapi/mux.go`
- Test: `server/restapi/restapi_test.go`

**Interfaces:**
- Consumes: `webhook.HealthMonitor.IsHealthy` — nhưng để tránh coupling restapi→webhook, `MuxConfig` nhận `ExtraHealthy func() bool` (nil = luôn healthy).
- Produces: `MuxConfig.ExtraHealthy func() bool`; `/healthz` trả 200 chỉ khi `pw.IsHealthy() && (ExtraHealthy == nil || ExtraHealthy())`.

- [ ] **Step 1: Write failing test**

Đọc `server/restapi/mux.go` và `restapi_test.go` để lấy đúng shape `MuxConfig`, `fakeModule`, helper `doReq`. Thêm test:

```go
func TestHealthz_ReflectsExtraHealthy(t *testing.T) {
	pw := &fakeModule{healthy: true}
	cfg := restapi.MuxConfig{APIKeys: []string{"k"}, ExtraHealthy: func() bool { return false }}
	srv := httptest.NewServer(restapi.NewAdminMux(pw, cfg))
	defer srv.Close()
	resp, _ := doReq(t, "GET", srv.URL+"/healthz", "", nil)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
```

(Nếu `doReq` trả khác, khớp theo chữ ký hiện có trong file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/restapi/ -run 'ReflectsExtraHealthy' -v`
Expected: FAIL (`ExtraHealthy` field chưa có).

- [ ] **Step 3: Add ExtraHealthy to MuxConfig and healthz**

Trong `server/restapi/mux.go`, thêm field vào `MuxConfig` và cập nhật handler `GET /healthz`:

```go
// trong struct MuxConfig:
ExtraHealthy func() bool

// trong handler healthz:
healthy := pw.IsHealthy() && (cfg.ExtraHealthy == nil || cfg.ExtraHealthy())
if healthy {
	// ... 200 branch hiện có
} else {
	// ... 503 branch hiện có
}
```

(Giữ nguyên body JSON `{"healthy":...}` shape hiện có.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/restapi/ -v`
Expected: PASS (test cũ + mới).

- [ ] **Step 5: Commit**

```bash
git add server/restapi/mux.go server/restapi/restapi_test.go
git commit -m "feat(restapi): healthz reflects extra health source (backend monitor)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 6: Wiring in main.go

**Files:**
- Modify: `cmd/pipewave-server/main.go`

**Interfaces:**
- Consumes: everything above — `serverconfig.CallbacksT` fields, `webhook.NewSyncCaller(sender, breaker, retryMax, backoff)`, `webhook.NewCircuitBreaker(threshold, cooldown)`, `webhook.NewAsyncDispatcher(sender, retryMax, backoff)`, `webhook.NewPinger`, `webhook.NewHealthMonitor`, `webhook.WatchBreakerOpen`, `restapi.MuxConfig.ExtraHealthy`.
- Produces: bootable binary với hành vi resilience mới. (No downstream task depends on internals.)

Đây là task wiring — không TDD unit; kiểm chứng bằng `go build` + `go vet` + boot smoke. Các unit đã có test riêng.

- [ ] **Step 1: Wire breaker + sync retry + async backoff from config**

Thay block khởi tạo webhook (`main.go` ~L51-53):

```go
	sender := webhook.NewSender(srvCfg.Callbacks.BaseURL, signer)

	asyncBackoff := webhook.DefaultBackoff
	if len(srvCfg.Callbacks.AsyncBackoff) > 0 {
		asyncBackoff = srvCfg.Callbacks.AsyncBackoff
	}
	async := webhook.NewAsyncDispatcher(sender, srvCfg.Callbacks.AsyncRetryMax, asyncBackoff)

	breaker := webhook.NewCircuitBreaker(srvCfg.Callbacks.Breaker.Threshold, srvCfg.Callbacks.Breaker.Cooldown)
	syncCaller := webhook.NewSyncCaller(sender, breaker,
		srvCfg.Callbacks.SyncRetry.Max, srvCfg.Callbacks.SyncRetry.Backoff)
```

- [ ] **Step 2: Build onUnhealthy + HealthMonitor before servers start**

Sau khi `rootCtx, stopSignals := signal.NotifyContext(...)` được tạo (di chuyển khối tạo rootCtx lên trước đoạn này nếu cần — nó hiện ở ~L60), thêm:

```go
	var unhealthyDueToBackend atomic.Bool
	onUnhealthy := func() {
		slog.Error("[pipewave-server] backend unhealthy (log-only)")
	}
	if srvCfg.Callbacks.UnhealthyAction == serverconfig.UnhealthyActionShutdown {
		onUnhealthy = func() {
			slog.Error("[pipewave-server] backend unhealthy — initiating shutdown")
			unhealthyDueToBackend.Store(true)
			stopSignals() // cancel rootCtx → tái dùng graceful shutdown path
		}
	}
	monitor := webhook.NewHealthMonitor(onUnhealthy)
```

Thêm import `"sync/atomic"`.

- [ ] **Step 3: Boot-check ping (fail-fast)**

Sau khi `sender` sẵn sàng và trước `go serve(...)`, thêm (chỉ khi ping enabled). Pinger dùng Sender riêng trỏ tới `BASE_URL + PING.PATH`:

```go
	var pinger *webhook.Pinger
	if srvCfg.Callbacks.Ping.Enabled {
		pingURL := srvCfg.Callbacks.BaseURL + srvCfg.Callbacks.Ping.Path
		pingSender := webhook.NewSender(pingURL, signer)
		pinger = webhook.NewPinger(pingSender, srvCfg.Callbacks.Ping.Timeout, srvCfg.Callbacks.Ping.FailThreshold)
		if srvCfg.Callbacks.Ping.BootCheck {
			bootCtx, cancel := context.WithTimeout(rootCtx, srvCfg.Callbacks.Ping.Timeout)
			err := pinger.Ping(bootCtx)
			cancel()
			if err != nil {
				fatal("callback ping", err)
			}
			slog.Info("[pipewave-server] callback ping OK")
		}
	}
```

- [ ] **Step 4: Start runtime pinger + breaker watcher goroutines**

Sau `go serve("admin", adminSrv)`:

```go
	if pinger != nil {
		go pinger.Run(rootCtx, srvCfg.Callbacks.Ping.Interval, monitor.SetHealthy,
			func() { monitor.SetUnhealthy("ping failed >= threshold") })
	}
	if srvCfg.Callbacks.BreakerOpenShutdown > 0 {
		go webhook.WatchBreakerOpen(rootCtx, breaker, srvCfg.Callbacks.BreakerOpenShutdown, monitor)
	}
```

- [ ] **Step 5: Wire monitor into healthz**

Trong đoạn build `muxCfg` cho adminSrv, thêm:

```go
	muxCfg.ExtraHealthy = monitor.IsHealthy
```

- [ ] **Step 6: Exit non-zero after drain if unhealthy triggered shutdown**

Ở cuối `main`, sau `slog.Info("[pipewave-server] bye")`:

```go
	if unhealthyDueToBackend.Load() {
		os.Exit(1)
	}
```

- [ ] **Step 7: Build + vet + smoke**

Run:
```bash
go build ./... && go vet ./cmd/... ./server/...
go test ./server/... ./cmd/...
```
Expected: build OK, vet clean, all tests PASS.

Boot smoke (log-only default, no ping): tạo config tối thiểu và chạy — server phải start bình thường như trước.

- [ ] **Step 8: Commit**

```bash
git add cmd/pipewave-server/main.go
git commit -m "feat(server): wire sync-retry, ping boot-check+runtime, breaker watcher, shutdown-on-unhealthy

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 7: Documentation

**Files:**
- Modify: `server/README.md`
- Modify: `server-config.example.yaml`

**Interfaces:** none (docs only).

- [ ] **Step 1: Update config table + failure semantics in README**

Trong `server/README.md`:
- Bảng config: thêm rows cho `CALLBACKS.SYNC_RETRY.MAX` (default `1`, "sync callback attempts; 1 = no retry"), `.SYNC_RETRY.BACKOFF` (`100ms`), `CALLBACKS.BREAKER.THRESHOLD` (`5`), `.BREAKER.COOLDOWN` (`10s`), `CALLBACKS.ASYNC_BACKOFF` (`[1s,5s,30s,2m,10m]`), `CALLBACKS.PING.*` (enabled `false`, path `/pipewave/ping`, interval `30s`, timeout `3s`, fail_threshold `3`), `CALLBACKS.UNHEALTHY_ACTION` (`log-only`), `CALLBACKS.BREAKER_OPEN_SHUTDOWN` (`0` = off).
- Failure semantics section: cập nhật dòng "No retry on sync callbacks" thành "sync callbacks retry transport/5xx up to `SYNC_RETRY.MAX` (default 1 = no retry), reusing one callback id; 4xx never retried". Ghi rõ worst-case client wait ≈ `MAX×timeout + (MAX-1)×backoff`.
- Thêm mục "Backend health & shutdown": mô tả ping (`event_type: "ping"`, expects 2xx), boot-check fatal, runtime ticker; `UNHEALTHY_ACTION=shutdown` graceful drain + exit 1; `log-only` → `/healthz` trả 503; `BREAKER_OPEN_SHUTDOWN`.

- [ ] **Step 2: Update example config**

Trong `server-config.example.yaml`, thêm dưới `SERVER.CALLBACKS` các key mới **có comment**, giữ ở default an toàn (ping disabled, unhealthy_action log-only) để ví dụ không đổi hành vi:

```yaml
    # ASYNC_BACKOFF: ["1s", "5s", "30s", "2m", "10m"]
    SYNC_RETRY:
      MAX: 1              # 1 = no retry (client is waiting)
      BACKOFF: "100ms"
    BREAKER:
      THRESHOLD: 5
      COOLDOWN: "10s"
    PING:
      ENABLED: false
      PATH: "/pipewave/ping"
      INTERVAL: "30s"
      TIMEOUT: "3s"
      FAIL_THRESHOLD: 3
    UNHEALTHY_ACTION: "log-only"   # or "shutdown"
    BREAKER_OPEN_SHUTDOWN: "0s"    # e.g. "60s" to exit when breaker stays open
```

- [ ] **Step 3: Verify docs build/lint (if any) and commit**

Run: `go test ./server/... ./cmd/...` (đảm bảo nothing broke; docs không ảnh hưởng test nhưng chạy để chắc chắn).

```bash
git add server/README.md server-config.example.yaml
git commit -m "docs(server): document callback resilience config + health/shutdown behavior

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review Notes

- **Spec coverage:** Config (Task 1) ✓; sync retry (Task 2) ✓; breaker params + OpenSince (Task 1 config, Task 2 method) ✓; Pinger boot+runtime (Task 3, wiring Task 6) ✓; HealthMonitor + breaker watcher (Task 4) ✓; healthz (Task 5) ✓; onUnhealthy/shutdown/log-only wiring (Task 6) ✓; docs (Task 7) ✓.
- **Signature consistency:** `NewSyncCaller(sender, breaker, retryMax, backoff)` used identically in Task 2 tests and Task 6 wiring. `NewPinger(sender, timeout, threshold)`, `NewHealthMonitor(onUnhealthy)`, `WatchBreakerOpen(ctx, b, maxOpen, m)`, `MuxConfig.ExtraHealthy func() bool` consistent across tasks.
- **Breaking change handled:** all 4 existing `NewSyncCaller` call-sites in `sync_test.go` updated in Task 2 Step 1; `main.go` in Task 6 Step 1.
- **Backward compat:** every new config defaults to current behavior (sync retryMax=1, ping disabled, unhealthy_action=log-only, breaker_open_shutdown=0).
- **BootCheck simplification:** documented as always-true-when-ping-enabled (not a config knob) to avoid koanf zero-value ambiguity — noted in Task 1 Step 4.
