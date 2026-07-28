# Callback Failure Resilience — Retry Config, Health-Ping & Shutdown-on-Unhealthy

**Date:** 2026-07-28
**Status:** Approved

---

## Overview

Khi backend callback endpoint fail, pipewave-server hiện chỉ log lại. Async path
đã có retry backoff + drop, sync path có circuit breaker — nhưng nhiều tham số bị
hardcode và không có cơ chế nào để server tự phản ứng khi backend "chết hẳn".

Thiết kế này bổ sung bốn nhóm năng lực, xoay quanh một khái niệm **"backend
health"** thống nhất:

1. **Config mở rộng** — đưa backoff schedule, circuit-breaker params, sync-retry,
   ping, và shutdown-policy vào file config.
2. **Sync retry** — `SyncCaller` retry giới hạn (nhanh) cho lỗi hạ tầng trước khi
   trả lỗi cho client.
3. **Pinger** — pipewave-server chủ động POST một `ping` event tới backend để kiểm
   tra endpoint còn sống (boot-check + ticker định kỳ).
4. **HealthMonitor** — hội tụ tín hiệu từ circuit breaker và pinger; khi backend bị
   coi là unhealthy quá lâu, kích hoạt shutdown hook.

**Nguyên tắc isolation cốt lõi:** package `webhook` KHÔNG biết gì về HTTP server
hay `os.Exit`. `HealthMonitor` chỉ nhận một callback `onUnhealthy func()`. Việc
wiring trong `main.go` mới quyết định callback đó làm gì (graceful drain + exit,
hoặc log-only). Điều này giữ package `webhook` testable thuần túy.

---

## Architecture

```
CircuitBreaker.Record(fail) ──► breaker-open watcher ──┐
                                                       ├──► HealthMonitor ──(unhealthy)──► onUnhealthy()
Pinger ticker (ping fail streak) ──────────────────────┘                                       │
                                                                          graceful drain + os.Exit(1)
```

| Unit                | Trách nhiệm                                                       | File                       |
|---------------------|------------------------------------------------------------------|----------------------------|
| **Config mở rộng**  | Backoff, breaker params, sync retry, ping, shutdown policy        | `server/config/config.go`  |
| **Sync retry**      | `SyncCaller` retry giới hạn cho lỗi hạ tầng                       | `server/webhook/sync.go`   |
| **Pinger**          | Chủ động POST `ping` event; boot-check + ticker                   | `server/webhook/ping.go` (mới) |
| **HealthMonitor**   | Nhận tín hiệu breaker + pinger; unhealthy quá lâu → shutdown hook | `server/webhook/health.go` (mới) |
| **Wiring**          | Ánh xạ `onUnhealthy` theo `UNHEALTHY_ACTION`; boot-check          | `cmd/pipewave-server/main.go` |
| **healthz**         | Phản ánh `monitor.IsHealthy()`                                    | `server/restapi/mux.go`    |

---

## 1. Config mở rộng

Thêm vào `CallbacksT` (`server/config/config.go`). **Mọi field mới đều có default
= hành vi hiện tại** — backward compatible.

```yaml
SERVER:
  CALLBACKS:
    # ... existing: BASE_URL, SIGNATURE, HANDLE_MESSAGE, SYNC_TIMEOUT, ASYNC_RETRY_MAX
    ASYNC_BACKOFF: ["1s", "5s", "30s", "2m", "10m"]   # schedule, last repeats
    SYNC_RETRY:
      MAX: 1              # tổng số attempt; 1 = no retry = hành vi cũ
      BACKOFF: "100ms"    # delay giữa các attempt; giữ nhỏ vì client đang đợi
    BREAKER:
      THRESHOLD: 5        # bỏ hardcode ở main.go
      COOLDOWN: "10s"
    PING:
      ENABLED: false           # opt-in
      PATH: "/pipewave/ping"   # ghép sau BASE_URL; rỗng = ping thẳng BASE_URL
      INTERVAL: "30s"
      TIMEOUT: "3s"
      FAIL_THRESHOLD: 3        # số ping fail liên tiếp trước khi coi unhealthy
      # Boot-check LUÔN chạy khi ping enabled (fail lúc boot → fatal, không start);
      # không phải knob cấu hình vì koanf không phân biệt unset/false cho bool.
    UNHEALTHY_ACTION: "log-only"   # shutdown | log-only
    BREAKER_OPEN_SHUTDOWN: "0s"    # breaker mở liên tục quá lâu → unhealthy (0 = tắt)
```

### Struct changes

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

### Constants

```go
const (
    UnhealthyActionShutdown = "shutdown"
    UnhealthyActionLogOnly  = "log-only"
)
```

### Defaults (an toàn, không đổi hành vi cũ)

Trong `loadDefault()`:

- `AsyncBackoff`: nếu rỗng, tầng wiring (`main.go`) truyền `webhook.DefaultBackoff`
  vào `NewAsyncDispatcher`; không cần điền default vào struct config.
- `SyncRetry.Max <= 0` → `1` (**sync vẫn no-retry** như hiện tại).
- `SyncRetry.Backoff <= 0` → `100ms`.
- `Breaker.Threshold <= 0` → `5`; `Breaker.Cooldown <= 0` → `10s`.
- `Ping.Enabled` mặc định `false`; nếu bật: `Path` mặc định `/pipewave/ping`,
  `Interval` <= 0 → `30s`, `Timeout` <= 0 → `3s`, `FailThreshold` <= 0 → `3`,
  `BootCheck` luôn set `true` (không phải knob — koanf không phân biệt unset/false).
- `UnhealthyAction == ""` → `"log-only"` (**mặc định KHÔNG tự exit** — service
  không nên tự giết mình trừ khi vận hành viên chủ động chọn `"shutdown"`).
- `BreakerOpenShutdown` mặc định `0` (tắt watcher).

### Validation

Trong `validate()`:

- `SyncRetry.Max >= 1` (sau default luôn thỏa).
- `UnhealthyAction ∈ {shutdown, log-only}`.
- `AsyncBackoff` mỗi phần tử parse được thành duration (koanf tự lo; validate
  non-negative). Nếu rỗng → dùng default, không lỗi.

---

## 2. Sync retry

Sửa `SyncCaller` (`server/webhook/sync.go`).

```go
type SyncCaller struct {
    sender   *Sender
    breaker  *CircuitBreaker
    retryMax int           // tổng số attempt; 1 = no retry
    backoff  time.Duration
}

func NewSyncCaller(sender *Sender, breaker *CircuitBreaker, retryMax int, backoff time.Duration) *SyncCaller
```

### Logic

`Call` sinh `callbackID := NewCallbackID()` **một lần** trước vòng lặp (tái dùng
cho mọi attempt → receiver dedupe được, giống async).

Vòng lặp tối đa `retryMax` attempt:

- `breaker.Allow()` false → trả `ErrCircuitOpen` ngay (fast-fail, **không** retry,
  không đợi backoff).
- Post thành công (2xx) → `breaker.Record(true)`, decode `out`, trả `nil`.
- **4xx** → `breaker.Record(true)` (4xx không phải lỗi hạ tầng), trả `*CallError`
  ngay — **KHÔNG retry** (đây là câu trả lời chủ ý của backend).
- **Transport error hoặc 5xx** → `breaker.Record(false)`. Nếu còn attempt và
  `ctx` chưa gần hết → chờ `backoff` rồi thử lại; nếu hết attempt → trả lỗi cuối.
- Mỗi attempt fail vẫn `Record(false)` riêng → một call fail 2 lần đếm 2 failures
  vào breaker (đúng: backend thực sự fail 2 lần).

### Timeout budget

Mỗi attempt dùng full per-call timeout (`SyncTimeout`/`HandleMessageTimeout`).
Toàn vòng lặp bao trong `ctx` gốc của caller: trước mỗi `backoff`/attempt kiểm
tra `ctx.Err()`; nếu ctx hết → cắt sớm, trả lỗi gần nhất.

Worst case client đợi ≈ `retryMax × timeout + (retryMax-1) × backoff`. Với default
`MAX=2, BACKOFF=100ms, timeout=3s` → ~6.1s. Documented rõ trong README + config
comment để vận hành viên set `MAX` có ý thức.

---

## 3. Pinger

Unit mới `server/webhook/ping.go`. Thêm `EventPing = "ping"` vào `envelope.go`.

Backend nhận POST giống mọi callback (signed envelope, `event_type: "ping"`, data
`{}`), chỉ cần trả 2xx.

```go
type Pinger struct {
    sender    *Sender       // Sender riêng trỏ tới BASE_URL + PING.PATH
    timeout   time.Duration
    threshold int           // ping fail liên tiếp → report unhealthy
}

func NewPinger(sender *Sender, timeout time.Duration, threshold int) *Pinger

// Ping một lần. Trả nil nếu 2xx. Dùng cho boot-check.
func (p *Pinger) Ping(ctx context.Context) error

// Run chạy ticker tới khi ctx hết. Mỗi fail tăng streak; đủ threshold gọi
// onUnhealthy() (guard 1 lần bởi HealthMonitor). Mỗi 2xx reset streak +
// gọi onHealthy().
func (p *Pinger) Run(ctx context.Context, interval time.Duration, onHealthy, onUnhealthy func())
```

### Sender riêng cho ping

Ping có thể cần path khác `BASE_URL`. Quyết định: tạo **một `Sender` thứ hai**
trỏ tới `BASE_URL + PING.PATH` (nếu `PATH` rỗng → thẳng `BASE_URL`), dùng chung
`signer`. Giữ `Sender` core và hot-path sync/async nguyên vẹn — không sửa chữ ký
`Sender.Post`.

### Boot-check

Trong `main.go`, nếu `PING.ENABLED && PING.BOOT_CHECK`: gọi `pinger.Ping(rootCtx)`
**trước** `serve(...)`. Fail → `fatal("callback ping", err)` — server không start.

### Runtime

`Run(...)` chạy trong goroutine riêng, dừng khi `rootCtx` hết (đăng ký cùng
shutdown sequence). Chỉ khởi động nếu `PING.ENABLED`.

---

## 4. HealthMonitor + wiring shutdown

Unit mới `server/webhook/health.go`.

```go
type HealthMonitor struct {
    mu          sync.Mutex
    healthy     bool
    onUnhealthy func()   // gọi ĐÚNG MỘT LẦN khi chuyển sang unhealthy
    fired       bool
}

func NewHealthMonitor(onUnhealthy func()) *HealthMonitor  // healthy=true ban đầu

func (m *HealthMonitor) SetHealthy()
func (m *HealthMonitor) SetUnhealthy(reason string)   // log CRITICAL; fire onUnhealthy 1 lần
func (m *HealthMonitor) IsHealthy() bool               // cho /healthz
```

### Hai nguồn feed vào monitor

**(a) Pinger** — `Run(...)` với
`onUnhealthy = func(){ monitor.SetUnhealthy("ping failed N times") }`,
`onHealthy = monitor.SetHealthy`.

**(b) Breaker-open-too-long watcher** — breaker hiện không có notion "mở bao lâu".
Thêm method trên `CircuitBreaker`:

```go
// OpenSince trả thời điểm breaker chuyển open nếu HIỆN ĐANG open, cùng ok=true.
func (b *CircuitBreaker) OpenSince() (time.Time, bool)
```

Watcher là một ticker nhẹ (mỗi 5s, hoặc `min(5s, cooldown)`): nếu breaker open và
`now - openSince >= BREAKER_OPEN_SHUTDOWN` → `monitor.SetUnhealthy("breaker open > …")`.
Nếu `BREAKER_OPEN_SHUTDOWN == 0` → **không chạy watcher**. Watcher dừng khi
`rootCtx` hết.

### Wiring trong main.go

`onUnhealthy` tùy `UNHEALTHY_ACTION`:

```go
var unhealthyDueToBackend atomic.Bool
onUnhealthy := func() {
    slog.Error("[pipewave-server] backend unhealthy — initiating shutdown")
    unhealthyDueToBackend.Store(true)
    stopSignals()   // cancel rootCtx → <-rootCtx.Done() unblocks → tái dùng graceful path
}
if srvCfg.Callbacks.UnhealthyAction == serverconfig.UnhealthyActionLogOnly {
    onUnhealthy = func() {
        slog.Error("[pipewave-server] backend unhealthy (log-only)")
        // healthz tự trả 503 nhờ monitor.IsHealthy()
    }
}
monitor := webhook.NewHealthMonitor(onUnhealthy)
```

`shutdown` mode chỉ gọi `stopSignals()` → tái dùng nguyên khối shutdown hiện có
(main.go graceful sequence). Ở cuối `main`, nếu `unhealthyDueToBackend.Load()` → `os.Exit(1)`
sau khi drain xong. Không nhân đôi shutdown logic.

`unhealthyDueToBackend` được ghi trong `onUnhealthy` (có thể từ goroutine khác) và
đọc ở cuối `main` sau khi drain hoàn tất — dùng `atomic.Bool` để tránh data race.

### healthz

`server/restapi/mux.go` (`GET /healthz`): đổi điều kiện thành
`pw.IsHealthy() && monitor.IsHealthy()`. Truyền `monitor` (hoặc một
`func() bool`) vào `NewAdminMux` / `MuxConfig`.

---

## Error handling & edge cases

- `onUnhealthy` fire **đúng 1 lần** (`fired` guard trong HealthMonitor) — tránh gọi
  `stopSignals` nhiều lần hay log spam khi cả ping lẫn breaker cùng báo unhealthy.
- Pinger + breaker-watcher goroutine dừng sạch khi `rootCtx` hết (shutdown thường).
- Boot-check fail → `fatal` khi chưa goroutine nào chạy → không leak.
- `log-only` mode: `/healthz` trả 503 nhưng process vẫn chạy; orchestrator tự quyết
  qua liveness probe.
- Sync retry tôn trọng `ctx` deadline → không kéo dài quá client timeout.

---

## Testing

- **`sync_test.go`**: retry đúng số lần cho 5xx/transport; KHÔNG retry 4xx/circuit-open;
  tái dùng callbackID qua các attempt; tôn trọng ctx deadline; breaker đếm mỗi
  attempt fail.
- **`ping_test.go`**: `Ping` map status→error (2xx=nil, non-2xx/transport=err); `Run`
  đếm streak, fire onUnhealthy khi đủ threshold, reset streak + onHealthy khi 2xx
  (short interval / fake clock).
- **`health_test.go`**: `SetUnhealthy` fire onUnhealthy đúng 1 lần dù gọi nhiều lần;
  `IsHealthy` phản ánh trạng thái; concurrency-safe.
- **`sync_test.go` (breaker)**: `OpenSince` trả đúng khi open / `ok=false` khi closed;
  watcher logic (unhealthy khi open quá ngưỡng).
- **`config_test.go`**: defaults mới (SyncRetry.Max=1, UnhealthyAction=log-only,
  ping defaults khi enabled); validation (Max>=1, action enum, backoff parse).

---

## Backward compatibility

Mọi field config mới có default = hành vi hiện tại:
- Sync path vẫn no-retry (`SYNC_RETRY.MAX=1`).
- Không ping (`PING.ENABLED=false`).
- Không tự shutdown (`UNHEALTHY_ACTION=log-only`, `BREAKER_OPEN_SHUTDOWN=0`).
- Breaker vẫn `5 / 10s` (giờ đọc từ config thay vì hardcode ở main.go).

Config file cũ chạy y hệt; các năng lực mới đều opt-in.
