# Pluggable Callback Transport (pubsub / NATS JetStream) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cho phép `pipewave-server` đẩy Class-2 (async, fire-and-forget) callback events qua pubsub (NATS JetStream) thay vì HTTP webhook, giữ webhook làm mặc định.

**Architecture:** Rút `async.Emit` ra sau một interface `AsyncTransport` nhỏ (3 method). `*webhook.AsyncDispatcher` hiện tại thoả interface đó sau khi thêm một method no-op, nên hành vi mặc định không đổi. Thêm implementation thứ hai đẩy qua `pkg/pubsub.Adapter` với adapter NATS JetStream mới. Class-1 (sync, request/reply) **không đụng tới** — vẫn là webhook.

**Tech Stack:** Go 1.25.5, `github.com/nats-io/nats.go` (mới), `github.com/stretchr/testify/require`, koanf config, Docker Compose.

## Global Constraints

- Module path: `github.com/pipewave-dev/go-pkg`. Go version: **1.25.5**.
- Test package convention: **external test package** (`package foo_test`), dùng `github.com/stretchr/testify/require`.
- Chạy test: `go test ./server/... ./pkg/pubsub/...`
- **`Emit` TUYỆT ĐỐI không được block** — nó được gọi từ WebSocket hot path. Mọi implementation phải dùng buffered channel + `select`/`default`.
- **Envelope không đổi**: dùng lại `webhook.Body` / `webhook.Meta` (`server/webhook/envelope.go`), cùng JSON marshalling. Không tạo envelope mới.
- Webhook là **default**. `TRANSPORT` rỗng ⟹ `webhook`. Không thay đổi hành vi người dùng hiện tại.
- `SERVER.CALLBACKS.BASE_URL` **vẫn luôn bắt buộc** ở mọi transport (Class-1 luôn dùng webhook).
- Không đụng `PubsubFactory` trong `cmd/pipewave-server/main.go` (đó là fanout nội bộ giữa các node, domain khác).
- Comment/doc trong repo này viết lẫn Anh/Việt — theo style file đang sửa.

---

### Task 1: `AsyncTransport` interface + `AsyncDispatcher` thoả interface

**Files:**
- Create: `server/callback/transport.go`
- Create: `server/callback/transport_test.go`
- Modify: `server/webhook/async.go` (thêm method `Healthcheck`)

**Interfaces:**
- Consumes: `*webhook.AsyncDispatcher` (có sẵn `Emit(eventType string, data any)`, `Shutdown(ctx context.Context)`).
- Produces: `callback.AsyncTransport` interface với 3 method: `Emit(eventType string, data any)`, `Healthcheck() error`, `Shutdown(ctx context.Context)`. Task 3, 4, 5 dùng interface này.

- [ ] **Step 1: Write the failing test**

Tạo `server/callback/transport_test.go`:

```go
package callback_test

import (
	"context"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/callback"
	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

// AsyncDispatcher phải thoả AsyncTransport để webhook vẫn là default
// mà không cần đổi gì ở fns.
func TestAsyncDispatcherSatisfiesAsyncTransport(t *testing.T) {
	d := webhook.NewAsyncDispatcher(
		webhook.NewSender("http://127.0.0.1:1", nil),
		1,
		[]time.Duration{time.Millisecond},
	)
	var tr callback.AsyncTransport = d
	require.NoError(t, tr.Healthcheck())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tr.Shutdown(ctx)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/callback/... -run TestAsyncDispatcherSatisfiesAsyncTransport -v`
Expected: FAIL — package `server/callback` chưa tồn tại (build error: no Go files / cannot find package).

- [ ] **Step 3: Write minimal implementation**

Tạo `server/callback/transport.go`:

```go
// Package callback định nghĩa transport contract cho việc đẩy callback
// events ra backend người dùng.
package callback

import "context"

// AsyncTransport delivers Class-2 (fire-and-forget) callback events.
//
// Emit MUST NOT block: nó được gọi từ WebSocket hot paths. Implementation
// phải buffer nội bộ và deliver trên goroutine riêng; khi buffer đầy thì
// drop kèm warning log, KHÔNG chặn caller.
type AsyncTransport interface {
	// Emit enqueues an event without blocking the caller.
	Emit(eventType string, data any)
	// Healthcheck reports transport health. Trả nil nghĩa là khoẻ.
	Healthcheck() error
	// Shutdown drains best-effort cho tới khi ctx hết hạn.
	Shutdown(ctx context.Context)
}
```

Thêm vào cuối `server/webhook/async.go`:

```go
// Healthcheck thoả callback.AsyncTransport. AsyncDispatcher không giữ
// kết nối dài hạn nào (mỗi lần deliver là một HTTP request riêng), nên
// sức khoẻ backend được theo dõi bởi Pinger, không phải ở đây.
func (d *AsyncDispatcher) Healthcheck() error { return nil }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/callback/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/callback/transport.go server/callback/transport_test.go server/webhook/async.go
git commit -m "feat(callback): add AsyncTransport interface satisfied by AsyncDispatcher"
```

---

### Task 2: Chuyển `fns` sang dùng `AsyncTransport`

**Files:**
- Modify: `server/fns/fns.go:105-110` (struct + `New`)
- Modify: `cmd/pipewave-server/main.go:133` (callsite)
- Test: `server/fns/fns_test.go` (thêm test dùng fake)

**Interfaces:**
- Consumes: `callback.AsyncTransport` (Task 1).
- Produces: `serverfns.New(syncCaller *webhook.SyncCaller, async callback.AsyncTransport, cfg Config) *types.Fns` — chữ ký đổi ở tham số thứ 2. Task 5 dùng chữ ký này.

- [ ] **Step 1: Write the failing test**

Thêm vào cuối `server/fns/fns_test.go`:

```go
// fakeAsync ghi lại các event Class-2 mà không cần HTTP server.
type fakeAsync struct {
	mu   sync.Mutex
	got  []string
	data []any
}

func (f *fakeAsync) Emit(eventType string, data any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, eventType)
	f.data = append(f.data, data)
}
func (f *fakeAsync) Healthcheck() error              { return nil }
func (f *fakeAsync) Shutdown(_ context.Context)      {}
func (f *fakeAsync) events() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got...)
}

// Class-2 hooks phải đi qua AsyncTransport, không phụ thuộc HTTP.
func TestAsyncHooksGoThroughTransport(t *testing.T) {
	fake := &fakeAsync{}
	fns := serverfns.New(nil, fake, serverfns.Config{
		HandleMessageMode: serverconfig.HandleMsgModeDisabled,
	})

	auth := types.WebsocketAuth{UserID: "u1", InstanceID: "i1"}
	fns.OnCloseConnection(context.Background(), auth)
	fns.OnReadError(context.Background(), auth, io.EOF)
	fns.OnWriteError(context.Background(), auth, io.EOF)

	require.Equal(t, []string{
		webhook.EventOnCloseConnection,
		webhook.EventOnReadError,
		webhook.EventOnWriteError,
	}, fake.events())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/fns/... -run TestAsyncHooksGoThroughTransport -v`
Expected: FAIL — compile error: không dùng `*fakeAsync` làm `*webhook.AsyncDispatcher` được.

- [ ] **Step 3: Write minimal implementation**

Trong `server/fns/fns.go`, thêm import `"github.com/pipewave-dev/go-pkg/server/callback"`, rồi đổi struct và `New`:

```go
type webhookFns struct {
	sync  *webhook.SyncCaller
	async callback.AsyncTransport
	cfg   Config
}

// New builds the *types.Fns that bridges pipewave hooks to callbacks.
// Class-1 (sync) luôn đi qua HTTP webhook; Class-2 (async) đi qua
// transport được truyền vào (webhook hoặc pubsub).
func New(syncCaller *webhook.SyncCaller, async callback.AsyncTransport, cfg Config) *types.Fns {
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
```

Trong `cmd/pipewave-server/main.go`, đổi khai báo ở dòng 71 để có kiểu interface:

```go
	var asyncTransport callback.AsyncTransport = webhook.NewAsyncDispatcher(
		sender, srvCfg.Callbacks.AsyncRetryMax, asyncBackoff)
```

Thêm import `"github.com/pipewave-dev/go-pkg/server/callback"`, rồi thay hai chỗ dùng biến `async`:
- dòng 133: `pw.SetFns(serverfns.New(syncCaller, asyncTransport, fnsCfg))`
- cuối hàm: `asyncTransport.Shutdown(shutdownCtx)`

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/... && go build ./...`
Expected: PASS, build sạch. Test cũ của `server/fns` và `server/webhook` vẫn xanh (không sửa chúng).

- [ ] **Step 5: Commit**

```bash
git add server/fns/fns.go server/fns/fns_test.go cmd/pipewave-server/main.go
git commit -m "refactor(fns): depend on AsyncTransport instead of concrete AsyncDispatcher"
```

---

### Task 3: Config `TRANSPORT` + `PUBSUB.*`

**Files:**
- Modify: `server/config/config.go` (constants, structs, `loadDefault`, `validate`)
- Test: `server/config/config_test.go`

**Interfaces:**
- Produces: `serverconfig.TransportWebhook = "webhook"`, `serverconfig.TransportPubsub = "pubsub"`, `serverconfig.PubsubDriverNATSJS = "natsjs"`; `CallbacksT.Transport string`, `CallbacksT.Pubsub CallbackPubsubT{Driver, URL, Stream, SubjectPrefix string}`. Task 5 đọc các field này.

- [ ] **Step 1: Write the failing test**

Thêm vào `server/config/config_test.go`:

```go
func TestLoad_TransportDefaultsToWebhook(t *testing.T) {
	cfg, err := serverconfig.Load([]string{writeYAML(t, validYAML)})
	require.NoError(t, err)
	require.Equal(t, serverconfig.TransportWebhook, cfg.Callbacks.Transport)
}

func TestLoad_PubsubDefaultsApplied(t *testing.T) {
	const y = `
SERVER:
  API_KEYS: ["key-1"]
  AUTH:
    MODE: "webhook"
  CALLBACKS:
    BASE_URL: "https://app.example.com/pipewave/callback"
    TRANSPORT: "pubsub"
    PUBSUB:
      URL: "nats://localhost:4222"
`
	cfg, err := serverconfig.Load([]string{writeYAML(t, y)})
	require.NoError(t, err)
	require.Equal(t, serverconfig.TransportPubsub, cfg.Callbacks.Transport)
	require.Equal(t, serverconfig.PubsubDriverNATSJS, cfg.Callbacks.Pubsub.Driver)
	require.Equal(t, "PIPEWAVE_EVENTS", cfg.Callbacks.Pubsub.Stream)
	require.Equal(t, "pipewave.events", cfg.Callbacks.Pubsub.SubjectPrefix)
}

func TestLoad_PubsubRequiresURL(t *testing.T) {
	const y = `
SERVER:
  API_KEYS: ["key-1"]
  AUTH:
    MODE: "webhook"
  CALLBACKS:
    BASE_URL: "https://app.example.com/pipewave/callback"
    TRANSPORT: "pubsub"
`
	_, err := serverconfig.Load([]string{writeYAML(t, y)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "PUBSUB.URL")
}

func TestLoad_RejectsUnknownTransport(t *testing.T) {
	const y = `
SERVER:
  API_KEYS: ["key-1"]
  AUTH:
    MODE: "webhook"
  CALLBACKS:
    BASE_URL: "https://app.example.com/pipewave/callback"
    TRANSPORT: "carrier-pigeon"
`
	_, err := serverconfig.Load([]string{writeYAML(t, y)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "TRANSPORT")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/config/... -run 'TestLoad_(Transport|Pubsub|RejectsUnknownTransport)' -v`
Expected: FAIL — compile error: `serverconfig.TransportWebhook` undefined.

- [ ] **Step 3: Write minimal implementation**

Trong `server/config/config.go`, thêm vào block `const` có sẵn:

```go
	TransportWebhook = "webhook"
	TransportPubsub  = "pubsub"

	PubsubDriverNATSJS = "natsjs"
```

Thêm hai field vào `CallbacksT` (giữ nguyên mọi field cũ):

```go
	Transport string          `koanf:"TRANSPORT"`
	Pubsub    CallbackPubsubT `koanf:"PUBSUB"`
```

Thêm struct mới:

```go
// CallbackPubsubT cấu hình transport pubsub cho Class-2 events. Đây là
// instance RIÊNG, không liên quan tới pubsub fanout nội bộ giữa các node.
type CallbackPubsubT struct {
	Driver        string `koanf:"DRIVER"`
	URL           string `koanf:"URL"`
	Stream        string `koanf:"STREAM"`
	SubjectPrefix string `koanf:"SUBJECT_PREFIX"`
}
```

Trong `loadDefault()`, thêm:

```go
	if c.Callbacks.Transport == "" {
		c.Callbacks.Transport = TransportWebhook
	}
	if c.Callbacks.Transport == TransportPubsub {
		if c.Callbacks.Pubsub.Driver == "" {
			c.Callbacks.Pubsub.Driver = PubsubDriverNATSJS
		}
		if c.Callbacks.Pubsub.Stream == "" {
			c.Callbacks.Pubsub.Stream = "PIPEWAVE_EVENTS"
		}
		if c.Callbacks.Pubsub.SubjectPrefix == "" {
			c.Callbacks.Pubsub.SubjectPrefix = "pipewave.events"
		}
	}
```

Trong `validate()`, thêm (BASE_URL check hiện có giữ nguyên):

```go
	switch c.Callbacks.Transport {
	case TransportWebhook:
	case TransportPubsub:
		if c.Callbacks.Pubsub.URL == "" {
			return fmt.Errorf("serverconfig: SERVER.CALLBACKS.PUBSUB.URL is required when TRANSPORT=pubsub")
		}
		if c.Callbacks.Pubsub.Driver != PubsubDriverNATSJS {
			return fmt.Errorf("serverconfig: SERVER.CALLBACKS.PUBSUB.DRIVER must be %q, got %q",
				PubsubDriverNATSJS, c.Callbacks.Pubsub.Driver)
		}
	default:
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.TRANSPORT must be %q or %q, got %q",
			TransportWebhook, TransportPubsub, c.Callbacks.Transport)
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/config/... -v`
Expected: PASS (cả test cũ — `TestLoad_DefaultsApplied` vẫn xanh).

- [ ] **Step 5: Commit**

```bash
git add server/config/config.go server/config/config_test.go
git commit -m "feat(config): add CALLBACKS.TRANSPORT and CALLBACKS.PUBSUB settings"
```

---

### Task 4: `pubsubTransport` — non-blocking Emit, subject mapping, envelope

**Files:**
- Create: `server/callback/pubsub.go`
- Create: `server/callback/pubsub_test.go`

**Interfaces:**
- Consumes: `callback.AsyncTransport` (Task 1); `pubsub.Adapter` từ `pkg/pubsub` (`Publish(ctx, channel string, message []byte) error`, `Healthcheck() error`, `Flush()`); `webhook.Body`/`webhook.Meta`/`webhook.NewCallbackID()`.
- Produces: `callback.NewPubsubTransport(pub Publisher, subjectPrefix string) *PubsubTransport` và interface `callback.Publisher`. Task 5 dùng.

**Lưu ý thiết kế:** `PubsubTransport` nhận một interface `Publisher` hẹp (chỉ `Publish`+`Healthcheck`) chứ không phải `pubsub.Adapter` đầy đủ, để test không phải implement `Subscribe`/`Flush`.

- [ ] **Step 1: Write the failing test**

Tạo `server/callback/pubsub_test.go`:

```go
package callback_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/callback"
	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

type capturedMsg struct {
	subject string
	payload []byte
}

// fakePub ghi lại message. blockUntil, nếu khác nil, chặn Publish cho tới
// khi nó được đóng — mô phỏng broker chết.
type fakePub struct {
	mu         sync.Mutex
	got        []capturedMsg
	blockUntil chan struct{}
}

func (f *fakePub) Publish(_ context.Context, subject string, payload []byte) error {
	if f.blockUntil != nil {
		<-f.blockUntil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, capturedMsg{subject: subject, payload: payload})
	return nil
}

func (f *fakePub) Healthcheck() error { return nil }

func (f *fakePub) messages() []capturedMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]capturedMsg(nil), f.got...)
}

func TestPubsubTransport_SubjectAndEnvelope(t *testing.T) {
	pub := &fakePub{}
	tr := callback.NewPubsubTransport(pub, "pipewave.events")

	tr.Emit(webhook.EventOnCloseConnection, map[string]string{"user_id": "u1"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tr.Shutdown(ctx)

	msgs := pub.messages()
	require.Len(t, msgs, 1)
	require.Equal(t, "pipewave.events.on_close_connection", msgs[0].subject)

	var env webhook.Body
	require.NoError(t, json.Unmarshal(msgs[0].payload, &env))
	require.Equal(t, webhook.EventOnCloseConnection, env.Meta.EventType)
	require.NotEmpty(t, env.Meta.CallbackID)
	require.NotZero(t, env.Meta.SentAt)
	require.JSONEq(t, `{"user_id":"u1"}`, string(env.Data))
}

// Test QUAN TRỌNG NHẤT: broker chết không được làm nghẽn WS hot path.
func TestPubsubTransport_EmitNeverBlocks(t *testing.T) {
	pub := &fakePub{blockUntil: make(chan struct{})}
	tr := callback.NewPubsubTransport(pub, "pipewave.events")

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Nhiều hơn sức chứa buffer — vẫn phải trả về ngay.
		for i := 0; i < 5000; i++ {
			tr.Emit(webhook.EventOnReadError, map[string]string{"n": "x"})
		}
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Emit blocked while broker was stalled")
	}

	close(pub.blockUntil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tr.Shutdown(ctx)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/callback/... -run TestPubsubTransport -v`
Expected: FAIL — `callback.NewPubsubTransport` undefined.

- [ ] **Step 3: Write minimal implementation**

Tạo `server/callback/pubsub.go`:

```go
package callback

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/pipewave-dev/go-pkg/server/webhook"
)

// queueSize khớp với AsyncDispatcher để hành vi backpressure giống nhau.
const queueSize = 1024

// Publisher là bề mặt hẹp mà PubsubTransport cần từ một pubsub adapter.
//
// HỢP ĐỒNG: Publish trả nil ⟹ broker đã DURABLE-ACK message. Adapter
// không đảm bảo được điều này (vd Redis/Valkey pub/sub: at-most-once,
// không ack) KHÔNG đủ tư cách làm callback transport.
type Publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
	Healthcheck() error
}

// PubsubTransport đẩy Class-2 events lên pubsub broker.
//
// Emit không bao giờ block: event vào buffered channel, một goroutine
// riêng publish. Queue đầy ⟹ drop kèm warning, giống AsyncDispatcher.
// Không retry in-memory — broker lo durability.
type PubsubTransport struct {
	pub           Publisher
	subjectPrefix string

	queue     chan pubsubJob
	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

type pubsubJob struct {
	subject string
	payload []byte
}

func NewPubsubTransport(pub Publisher, subjectPrefix string) *PubsubTransport {
	t := &PubsubTransport{
		pub:           pub,
		subjectPrefix: subjectPrefix,
		queue:         make(chan pubsubJob, queueSize),
		closed:        make(chan struct{}),
	}
	t.wg.Add(1)
	go t.loop()
	return t
}

func (t *PubsubTransport) Emit(eventType string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		slog.Error("[callback/pubsub] marshal data", "event_type", eventType, "error", err)
		return
	}
	payload, err := json.Marshal(webhook.Body{
		Data: raw,
		Meta: webhook.Meta{
			SentAt:     nowUnixMilli(),
			CallbackID: webhook.NewCallbackID(),
			EventType:  eventType,
		},
	})
	if err != nil {
		slog.Error("[callback/pubsub] marshal envelope", "event_type", eventType, "error", err)
		return
	}

	select {
	case t.queue <- pubsubJob{subject: t.subjectPrefix + "." + eventType, payload: payload}:
	default:
		slog.Warn("[callback/pubsub] queue full, dropping event", "event_type", eventType)
	}
}

func (t *PubsubTransport) Healthcheck() error { return t.pub.Healthcheck() }

func (t *PubsubTransport) Shutdown(ctx context.Context) {
	t.closeOnce.Do(func() { close(t.closed) })
	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (t *PubsubTransport) loop() {
	defer t.wg.Done()
	for {
		select {
		case job := <-t.queue:
			t.deliver(job)
		case <-t.closed:
			for {
				select {
				case job := <-t.queue:
					t.deliver(job)
				default:
					return
				}
			}
		}
	}
}

func (t *PubsubTransport) deliver(job pubsubJob) {
	if err := t.pub.Publish(context.Background(), job.subject, job.payload); err != nil {
		slog.Warn("[callback/pubsub] publish failed", "subject", job.subject, "error", err)
	}
}
```

Thêm helper vào cùng file (tách ra để test không phụ thuộc đồng hồ thật nếu sau này cần):

```go
func nowUnixMilli() int64 { return time.Now().UnixMilli() }
```

(nhớ thêm `"time"` vào import)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/callback/... -race -v`
Expected: PASS cả hai test, không có race.

- [ ] **Step 5: Commit**

```bash
git add server/callback/pubsub.go server/callback/pubsub_test.go
git commit -m "feat(callback): add non-blocking PubsubTransport for Class-2 events"
```

---

### Task 5: NATS JetStream adapter + wiring `main.go`

**Files:**
- Create: `pkg/pubsub/adapters/natsjs/instance.go`
- Create: `pkg/pubsub/adapters/natsjs/instance_test.go`
- Modify: `cmd/pipewave-server/main.go`
- Modify: `go.mod` / `go.sum`

**Interfaces:**
- Consumes: `callback.Publisher` (Task 4), `callback.NewPubsubTransport` (Task 4), `serverconfig.TransportPubsub` + `CallbackPubsubT` (Task 3).
- Produces: `natsjs.New(cfg *natsjs.Config) (*natsjs.Adapter, error)` với `Config{URL, Stream, SubjectPrefix string}`.

- [ ] **Step 1: Write the failing test**

Tạo `pkg/pubsub/adapters/natsjs/instance_test.go` — test không cần broker thật, chỉ khẳng định contract và lỗi kết nối:

```go
package natsjs_test

import (
	"context"
	"testing"

	"github.com/pipewave-dev/go-pkg/pkg/pubsub/adapters/natsjs"
	"github.com/stretchr/testify/require"
)

// Adapter phải thoả bề mặt Publisher mà server/callback yêu cầu.
//
// CỐ Ý khai báo lại interface tại đây thay vì import server/callback:
// pkg/ không được phụ thuộc server/ (hướng phụ thuộc đi một chiều
// server/ -> pkg/). Structural typing của Go vẫn cho ta kiểm tra
// contract này lúc compile.
type publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
	Healthcheck() error
}

func TestAdapterSatisfiesPublisher(t *testing.T) {
	var _ publisher = (*natsjs.Adapter)(nil)
}

// Không kết nối được thì phải lỗi ngay lúc New, không phải im lặng.
func TestNewFailsOnUnreachableBroker(t *testing.T) {
	_, err := natsjs.New(&natsjs.Config{
		URL:    "nats://127.0.0.1:1",
		Stream: "TEST_STREAM",
	})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/pubsub/adapters/natsjs/... -v`
Expected: FAIL — package chưa tồn tại.

- [ ] **Step 3: Write minimal implementation**

Thêm dependency:

```bash
go get github.com/nats-io/nats.go@latest
```

Tạo `pkg/pubsub/adapters/natsjs/instance.go`:

```go
// Package natsjs cung cấp một pubsub publisher dựa trên NATS JetStream.
//
// Khác với adapter Valkey (at-most-once, không ack), JetStream ack sau khi
// message đã persist — đủ điều kiện làm callback transport.
package natsjs

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Config struct {
	URL           string
	Stream        string
	SubjectPrefix string
}

type Adapter struct {
	conn *nats.Conn
	js   jetstream.JetStream
}

// New kết nối tới NATS, bật JetStream và đảm bảo stream tồn tại
// (idempotent: tạo nếu chưa có, cập nhật subject nếu đã có).
func New(cfg *Config) (*Adapter, error) {
	conn, err := nats.Connect(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("natsjs: connect %s: %w", cfg.URL, err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("natsjs: jetstream: %w", err)
	}

	if cfg.Stream != "" {
		subjects := []string{cfg.SubjectPrefix + ".>"}
		if cfg.SubjectPrefix == "" {
			subjects = []string{cfg.Stream + ".>"}
		}
		_, err = js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
			Name:     cfg.Stream,
			Subjects: subjects,
			Storage:  jetstream.FileStorage,
		})
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("natsjs: ensure stream %s: %w", cfg.Stream, err)
		}
	}

	return &Adapter{conn: conn, js: js}, nil
}

// Publish gửi message và CHỜ JetStream ack — trả nil nghĩa là đã persist.
// Msg-Id được set từ callback ID trong envelope để JetStream dedupe khi
// có retry.
func (a *Adapter) Publish(ctx context.Context, subject string, payload []byte) error {
	msg := &nats.Msg{Subject: subject, Data: payload}
	if id := callbackIDFrom(payload); id != "" {
		msg.Header = nats.Header{"Nats-Msg-Id": []string{id}}
	}
	if _, err := a.js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("natsjs: publish %s: %w", subject, err)
	}
	return nil
}

func (a *Adapter) Healthcheck() error {
	if !a.conn.IsConnected() {
		return fmt.Errorf("natsjs: not connected")
	}
	return nil
}

func (a *Adapter) Close() {
	if a.conn != nil {
		a.conn.Close()
	}
}
```

Thêm helper đọc `meta.id` từ envelope (cùng file):

```go
// callbackIDFrom rút meta.id khỏi envelope để dùng làm Nats-Msg-Id.
// Lỗi parse trả "" — publish vẫn tiếp tục, chỉ mất khả năng dedupe.
func callbackIDFrom(payload []byte) string {
	var env struct {
		Meta struct {
			ID string `json:"id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return ""
	}
	return env.Meta.ID
}
```

(thêm `"encoding/json"` vào import)

Trong `cmd/pipewave-server/main.go`, thay dòng khai báo `asyncTransport` (từ Task 2) bằng nhánh theo transport:

```go
	var asyncTransport callback.AsyncTransport
	switch srvCfg.Callbacks.Transport {
	case serverconfig.TransportPubsub:
		nc, npErr := natsjs.New(&natsjs.Config{
			URL:           srvCfg.Callbacks.Pubsub.URL,
			Stream:        srvCfg.Callbacks.Pubsub.Stream,
			SubjectPrefix: srvCfg.Callbacks.Pubsub.SubjectPrefix,
		})
		if npErr != nil {
			fatal("init callback pubsub transport", npErr)
		}
		asyncTransport = callback.NewPubsubTransport(nc, srvCfg.Callbacks.Pubsub.SubjectPrefix)
		slog.Info("[pipewave-server] async callbacks via pubsub",
			"driver", srvCfg.Callbacks.Pubsub.Driver, "url", srvCfg.Callbacks.Pubsub.URL)
	default:
		asyncTransport = webhook.NewAsyncDispatcher(sender, srvCfg.Callbacks.AsyncRetryMax, asyncBackoff)
	}
```

Thêm import `natsjs "github.com/pipewave-dev/go-pkg/pkg/pubsub/adapters/natsjs"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./pkg/pubsub/... ./server/... -v`
Expected: PASS, build sạch.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum pkg/pubsub/adapters/natsjs cmd/pipewave-server/main.go
git commit -m "feat(pubsub): add NATS JetStream adapter and wire pubsub transport"
```

---

### Task 6: Docker compose + example backend (end-to-end)

**Files:**
- Modify: `docker-compose.yml`
- Create: `examples/pubsub-backend/main.go`
- Create: `examples/pubsub-backend/README.md`

**Interfaces:**
- Consumes: envelope JSON `{"data":..., "meta":{"sent_at","id","event_type"}}` (Task 4); subject `pipewave.events.<event_type>` (Task 4); config keys từ Task 3.

**Lưu ý:** `docker-compose.yml` có comment ghi rõ env override `APP_`-prefix đang lỗi với nested field — **phải** override qua file config thứ hai (`configs:`), không dùng env var.

- [ ] **Step 1: Thêm service NATS vào `docker-compose.yml`**

Thêm vào `services:` (giữ nguyên network `pipewave`):

```yaml
  nats:
    image: nats:2-alpine
    command: ["-js", "-m", "8222"]
    ports:
      - "29103:4222"
      - "29104:8222"
    networks:
      - pipewave
```

Thêm `nats` vào `depends_on` của `pipewave-server`:

```yaml
    depends_on:
      - valkey
      - postgres
      - nats
```

Thêm vào `configs.pipewave-docker-overrides.content`, dưới `SERVER.CALLBACKS` (giữ nguyên `BASE_URL` — Class-1 vẫn cần):

```yaml
          TRANSPORT: "pubsub"
          PUBSUB:
            URL: "nats://nats:4222"
```

- [ ] **Step 2: Viết example backend**

Tạo `examples/pubsub-backend/main.go` — subscribe Class-2 qua NATS, **vẫn phục vụ HTTP cho Class-1** (chính điều này chứng minh hybrid chạy thật):

```go
// Command pubsub-backend là backend mẫu cho chế độ hybrid:
//   - Class-2 (async) nhận qua NATS JetStream.
//   - Class-1 (sync) vẫn nhận qua HTTP webhook, vì chúng cần response.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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
	natsURL := envOr("NATS_URL", "nats://localhost:29103")
	addr := envOr("ADDR", ":9000")

	go serveClass1(addr)

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("connect nats: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	ctx := context.Background()
	cons, err := js.CreateOrUpdateConsumer(ctx, "PIPEWAVE_EVENTS", jetstream.ConsumerConfig{
		Durable:       "pubsub-backend",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: "pipewave.events.>",
	})
	if err != nil {
		log.Fatalf("create consumer: %v", err)
	}

	log.Printf("subscribed to pipewave.events.> on %s", natsURL)
	_, err = cons.Consume(func(msg jetstream.Msg) {
		var env envelope
		if err := json.Unmarshal(msg.Data(), &env); err != nil {
			log.Printf("bad envelope on %s: %v", msg.Subject(), err)
			_ = msg.Ack() // không nak: message hỏng, retry cũng vô ích
			return
		}
		switch env.Meta.EventType {
		case "on_new_connection_established", "on_close_connection",
			"on_read_error", "on_write_error", "message_received":
			log.Printf("event=%s id=%s data=%s", env.Meta.EventType, env.Meta.ID, env.Data)
		default:
			log.Printf("unhandled event=%s id=%s", env.Meta.EventType, env.Meta.ID)
		}
		_ = msg.Ack()
	})
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	select {}
}

// serveClass1 phục vụ các callback sync (inspect_token, handle_message,
// on_new_connection) — pubsub KHÔNG thay thế được vì chúng cần response.
func serveClass1(addr string) {
	http.HandleFunc("/pipewave/callback", func(w http.ResponseWriter, r *http.Request) {
		var env envelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch env.Meta.EventType {
		case "inspect_token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user_id": "demo-user", "is_anonymous": false,
				"metadata": map[string]string{},
			})
		case "handle_message":
			// echo lại nguyên payload
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output_type": "text", "data": []byte("pong"),
			})
		default: // on_new_connection: 2xx là chấp nhận kết nối
			w.WriteHeader(http.StatusOK)
		}
	})
	log.Printf("class-1 webhook listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("http: %v", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 3: Viết README**

Tạo `examples/pubsub-backend/README.md`:

```markdown
# pubsub-backend

Backend mẫu cho chế độ **hybrid**:

- **Class-2** (`on_close_connection`, `on_read_error`, `on_write_error`,
  `message_received`, `on_new_connection_established`) — nhận qua **NATS JetStream**,
  subject `pipewave.events.>`.
- **Class-1** (`inspect_token`, `handle_message`, `on_new_connection`) — vẫn nhận
  qua **HTTP webhook**, vì chúng cần giá trị trả về trên hot path.

## Chạy

```bash
docker compose up -d nats postgres valkey
go run ./examples/pubsub-backend      # lắng nghe :9000 + subscribe NATS
docker compose up pipewave-server
```

Env: `NATS_URL` (mặc định `nats://localhost:29103`), `ADDR` (mặc định `:9000`).
```

- [ ] **Step 4: Verify end-to-end**

Run:
```bash
go build ./... && go vet ./...
docker compose up -d nats
go run ./examples/pubsub-backend &
docker compose up --build pipewave-server
```

Expected: `pipewave-server` log `async callbacks via pubsub`; khi có client kết nối rồi ngắt, `pubsub-backend` in ra `event=on_close_connection id=cb_...`.

Nếu không có client thật để thử, tối thiểu phải xác nhận: server khởi động sạch với `TRANSPORT: pubsub`, stream `PIPEWAVE_EVENTS` được tạo (`docker compose exec nats nats stream ls` hoặc xem monitoring port 29104).

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml examples/pubsub-backend
git commit -m "feat(examples): add pubsub-backend example and NATS service in compose"
```

---

### Task 7: Tài liệu cấu hình

**Files:**
- Modify: `README.md` (mục cấu hình `SERVER.CALLBACKS`)
- Modify: `server-config.example.yaml`

**Interfaces:**
- Consumes: config keys từ Task 3.

- [ ] **Step 1: Thêm block pubsub (đã comment) vào `server-config.example.yaml`**

Dưới `SERVER.CALLBACKS`, thêm:

```yaml
    # Transport cho Class-2 (async) events: webhook | pubsub. Mặc định webhook.
    # Class-1 (inspect_token, handle_message, on_new_connection) LUÔN dùng
    # webhook vì cần response — vì vậy BASE_URL vẫn bắt buộc ở mọi transport.
    TRANSPORT: "webhook"
    # PUBSUB:
    #   DRIVER: "natsjs"
    #   URL: "nats://localhost:4222"
    #   STREAM: "PIPEWAVE_EVENTS"
    #   SUBJECT_PREFIX: "pipewave.events"
```

- [ ] **Step 2: Ghi tài liệu trong `README.md`**

Tìm mục mô tả `SERVER.CALLBACKS` và thêm phần con:

```markdown
#### Callback transport (webhook / pubsub)

Events chia hai lớp:

| Lớp | Events | Transport |
|-----|--------|-----------|
| Class-1 (sync, cần response) | `inspect_token`, `handle_message`, `on_new_connection` | **luôn webhook** |
| Class-2 (async, fire-and-forget) | `on_new_connection_established`, `on_close_connection`, `on_read_error`, `on_write_error`, `message_received` | `webhook` (mặc định) hoặc `pubsub` |

Đặt `SERVER.CALLBACKS.TRANSPORT: pubsub` để đẩy Class-2 qua NATS JetStream.
Mỗi event vào subject `{SUBJECT_PREFIX}.{event_type}`, ví dụ
`pipewave.events.on_close_connection`; backend subscribe wildcard
`pipewave.events.>`. Envelope **giống hệt** webhook, nên code parse dùng lại được.

Ở chế độ `pubsub`:
- **Chữ ký Ed25519 không áp dụng** cho Class-2 (auth thuộc tầng broker); Class-1 vẫn ký.
- **Ping/CircuitBreaker** chỉ còn tác dụng với Class-1.
- `meta.id` được set làm `Nats-Msg-Id` để JetStream dedupe.

Xem `examples/pubsub-backend` để có backend chạy được.
```

- [ ] **Step 3: Verify**

Run: `docker compose config >/dev/null && go build ./...`
Expected: compose hợp lệ, build sạch.

- [ ] **Step 4: Commit**

```bash
git add README.md server-config.example.yaml
git commit -m "docs: document callback transport modes (webhook/pubsub)"
```

---

## Ghi chú cho người thực thi

- **Không** sửa test hiện có trong `server/webhook/` — chúng phải xanh nguyên vẹn; đó là bằng chứng webhook vẫn là default không đổi hành vi.
- Nếu `go get nats.go` kéo về API khác với `jetstream.New(...)` dùng trong Task 5, ưu tiên API thật của thư viện và giữ nguyên **hợp đồng**: `Publish` phải chờ ack, `New` phải lỗi ngay khi không kết nối được.
- Chạy `go test ./... -race` trước khi kết thúc — `PubsubTransport` có goroutine + shared state.
