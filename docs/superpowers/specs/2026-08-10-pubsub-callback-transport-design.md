# Design: Pluggable callback transport — pubsub (NATS JetStream) cho Class-2 events

**Ngày:** 2026-08-10
**Trạng thái:** Approved (chờ user review spec)

## Bối cảnh

`pipewave-server` (server-mode, chạy như container riêng) hiện chỉ có **một** cách
đẩy event ra backend người dùng: HTTP webhook callback tới `SERVER.CALLBACKS.BASE_URL`.
Yêu cầu: hỗ trợ thêm liên kết qua pubsub (NATS, Kafka) như một lựa chọn thay thế.

Thiết kế này giải quyết yêu cầu đó theo hướng **hybrid**: chỉ Class-2 (async,
fire-and-forget) chuyển được sang pubsub; Class-1 (sync, request/reply) giữ nguyên
webhook.

## Hiện trạng (code)

### Hai class event đã được phân loại sẵn

`server/webhook/envelope.go:11-13` ghi rõ:

- **Class-1** (sync, cần response): `inspect_token`, `handle_message`, `on_new_connection`.
- **Class-2** (async, fire-and-forget + retry): `on_new_connection_established`,
  `on_close_connection`, `on_read_error`, `on_write_error`, `message_received`, `ping`.

### Điểm chèn đã rất gọn

`webhookFns` (`server/fns/fns.go:107`) chỉ phụ thuộc hai thứ:

```go
type webhookFns struct {
    sync  *webhook.SyncCaller
    async *webhook.AsyncDispatcher
    cfg   Config
}
```

Bề mặt thực sự được dùng chỉ gồm **hai method**:

- `w.sync.Call(ctx, eventType, data, timeout, out)` — Class-1, có giá trị trả về.
- `w.async.Emit(eventType, data)` — Class-2, không trả gì.

### Vì sao Class-1 không chuyển sang pubsub được (một cách tự nhiên)

Cả ba Class-1 event đều chặn hot path **và cần giá trị trả về**:

- `inspectToken` (`fns.go:132`) đọc `resp.UserID`, `resp.IsAnonymous`, `resp.Metadata`.
- `HandleMessage` (`fns.go:150`) đọc `resp.OutputType`, `resp.Data` để đẩy ngược về client.
- `OnNewConnection` (`fns.go:161`) fail-closed: chỉ 2xx mới cho phép kết nối.

Pubsub không cho request/reply một cách tự nhiên. NATS có request/reply first-class,
nhưng Kafka thì phải tự dựng correlation-ID + reply topic + pending-request map —
chống lại thiết kế của công cụ, và rủi ro latency trên hot path.

**Quyết định: Class-1 giữ nguyên webhook, không đụng tới trong scope này.**

### Vấn đề có thật mà pubsub giải quyết

`AsyncDispatcher` (`server/webhook/async.go:32-35`) tự nhận trong doc comment rằng nó
**drop event**:

- khi queue đầy (`asyncQueueSize = 1024`, `async.go:63-68`),
- khi hết `retryMax` (`async.go:143`),
- **khi shutdown/crash** — "accepted for v1", và trong trường hợp đó có thể drop
  *không cả log warning*.

Retry là in-memory, mất khi process chết. Chuyển Class-2 sang broker durable không chỉ
là "thêm option" — nó sửa một điểm mất dữ liệu có thật.

### `pkg/pubsub` hiện có

`pkg/pubsub/interface.go`:

```go
type Adapter interface {
    Publish(ctx context.Context, channel string, message []byte) error
    Subscribe(channel string, handler func(message []byte)) (unsubscribe func(), err error)
    Healthcheck() error
    Flush()
}
```

Interface này **đúng hình dạng cần dùng**. Nhưng hiện nó đang phục vụ **fanout nội bộ
giữa các node pipewave** qua Valkey (`PubsubFactory` trong `cmd/pipewave-server/main.go:44`),
là domain khác hẳn với callback transport ra backend. Adapter duy nhất hiện có
(Valkey/Redis pub/sub) là **at-most-once**: không ack, không replay — đúng cái bệnh
của `AsyncDispatcher` mà ta đang đi chữa.

## Quyết định thiết kế

### 1. Tách `AsyncTransport` interface

File mới `server/callback/transport.go`:

```go
// AsyncTransport delivers Class-2 (fire-and-forget) callback events.
//
// Emit MUST NOT block: it is called from WebSocket hot paths. Implementations
// buffer internally and deliver on their own goroutine.
type AsyncTransport interface {
    Emit(eventType string, data any)
    Healthcheck() error
    Shutdown(ctx context.Context)
}
```

`webhookFns.async` đổi kiểu từ `*webhook.AsyncDispatcher` → `AsyncTransport`.
`serverfns.New` đổi tham số tương ứng.

`*webhook.AsyncDispatcher` **đã thoả** interface này (`Emit`, `Shutdown` có sẵn);
chỉ cần thêm `Healthcheck() error { return nil }`.

**Hệ quả: zero thay đổi hành vi cho người dùng hiện tại.** Webhook vẫn là default.

`sync.Call` giữ nguyên kiểu `*webhook.SyncCaller` — không trừu tượng hoá, vì Class-1
ngoài scope.

### 2. Tái sử dụng `pkg/pubsub.Adapter`, siết hợp đồng

Không định nghĩa interface pubsub mới. Dùng lại `pubsub.Adapter` làm contract, và bổ
sung doc comment siết **ngữ nghĩa**:

> `Publish` trả `nil` ⟹ broker đã **durable-ack** message. Adapter không đảm bảo được
> điều này (Redis/Valkey pub/sub: at-most-once, không ack) **không đủ tư cách** làm
> callback transport, dù code vẫn chạy.

Chữ ký interface **không đổi** — chỉ hợp đồng chặt hơn. `Subscribe` không dùng ở đường
này (server chỉ publish; backend mới subscribe).

Adapter mới: `pkg/pubsub/adapters/natsjs` (NATS JetStream, publish có ack).

### 3. Instance + config riêng, không lẫn với pubsub nội bộ

Callback transport dùng **instance riêng, config riêng**, có thể trỏ tới broker khác
hẳn Valkey nội bộ:

```yaml
SERVER:
  CALLBACKS:
    TRANSPORT: webhook          # webhook | pubsub   (default: webhook)
    BASE_URL: "..."             # bắt buộc (Class-1 luôn dùng webhook)
    PUBSUB:
      DRIVER: natsjs            # chỉ natsjs trong đợt này
      URL: "nats://nats:4222"
      STREAM: PIPEWAVE_EVENTS
      SUBJECT_PREFIX: pipewave.events
```

Thêm vào `server/config/config.go`:

```go
const (
    TransportWebhook = "webhook"
    TransportPubsub  = "pubsub"

    PubsubDriverNATSJS = "natsjs"
)

type CallbacksT struct {
    // ... các field hiện có giữ nguyên
    Transport string           `koanf:"TRANSPORT"`
    Pubsub    CallbackPubsubT  `koanf:"PUBSUB"`
}

type CallbackPubsubT struct {
    Driver        string `koanf:"DRIVER"`
    URL           string `koanf:"URL"`
    Stream        string `koanf:"STREAM"`
    SubjectPrefix string `koanf:"SUBJECT_PREFIX"`
}
```

**Defaults** (`loadDefault`): `Transport` rỗng → `webhook`. Khi `Transport == pubsub`:
`Driver` rỗng → `natsjs`, `Stream` rỗng → `PIPEWAVE_EVENTS`, `SubjectPrefix` rỗng →
`pipewave.events`.

**Validate** (`validate`):

- `TRANSPORT` phải là `webhook|pubsub`.
- `TRANSPORT=pubsub` ⟹ `PUBSUB.URL` bắt buộc, `PUBSUB.DRIVER` phải là `natsjs`.
- `BASE_URL` **vẫn luôn bắt buộc** (Class-1 dùng webhook trong mọi mode) — giữ nguyên
  rule hiện có ở `config.go:validate`.

### 4. Subject mapping và envelope

Mỗi event type → một subject riêng: `{SUBJECT_PREFIX}.{event_type}`, ví dụ
`pipewave.events.on_close_connection`.

Backend subscribe wildcard `pipewave.events.>` hoặc lọc từng loại. Đây là lợi ích thực
sự so với webhook: routing + nhiều consumer độc lập, thay vì một endpoint HTTP duy nhất.

**Envelope giữ nguyên y hệt webhook** (`server/webhook/envelope.go`):

```json
{ "data": {...}, "meta": { "sent_at": 0, "id": "cb_...", "event_type": "..." } }
```

Cùng struct `webhook.Body`, cùng JSON marshalling. Backend đang xử lý webhook tái dùng
được nguyên code parse — chỉ đổi cách nhận.

`Meta.CallbackID` (`cb_...`) được set làm **NATS `Msg-Id` header** để JetStream dedupe
(at-least-once + idempotency key, giống hệt vai trò của nó ở webhook retry).

### 5. `Emit` không bao giờ block

Ràng buộc quan trọng nhất. `AsyncDispatcher.Emit` hiện non-blocking tuyệt đối
(`async.go:63-68`, select với `default:`), và WS hot path gọi nó. NATS publish có thể
block khi mất kết nối.

`pubsubTransport` **phải giữ nguyên hình dạng đó**: buffered channel + goroutine
publisher; `Emit` dùng `select ... default:` drop-with-warning y như `AsyncDispatcher`.
Broker chậm **không được** làm nghẽn WebSocket.

Publish lỗi → log + đếm metric. Không tự retry in-memory: JetStream + NATS client
reconnect buffer lo phần đó.

### 6. Signature / Ping / Breaker trong pubsub mode

- **Signature** (Ed25519): không áp dụng cho Class-2 khi ở pubsub mode — auth thuộc
  tầng broker. Class-1 (webhook) vẫn ký như cũ. Không xoá code.
- **Ping/Pinger**: `pinger` hiện health-check `BASE_URL + PATH`. Vẫn giữ nguyên cho
  Class-1. Sức khoẻ pubsub dùng `AsyncTransport.Healthcheck()`.
- **CircuitBreaker**: chỉ dùng cho `SyncCaller` (Class-1) — không đổi.

Nêu rõ trong config docs, không xoá code nào.

### 7. Wiring trong `main.go`

Thay đoạn tạo `async` (`cmd/pipewave-server/main.go`) bằng nhánh theo `Transport`:

```go
var asyncTransport callback.AsyncTransport
switch srvCfg.Callbacks.Transport {
case serverconfig.TransportPubsub:
    asyncTransport, err = callback.NewPubsubTransport(...)  // natsjs adapter
    if err != nil { fatal("init callback pubsub transport", err) }
default:
    asyncTransport = webhook.NewAsyncDispatcher(sender, ..., asyncBackoff)
}
```

`pw.SetFns(serverfns.New(syncCaller, asyncTransport, fnsCfg))` và
`asyncTransport.Shutdown(shutdownCtx)` ở cuối — thay cho `async.Shutdown`.

## Phạm vi (scope)

**Trong scope:**

- `AsyncTransport` interface + refactor `fns` sang dùng nó.
- Adapter `pkg/pubsub/adapters/natsjs` (JetStream, durable publish ack).
- `pubsubTransport` (non-blocking Emit, subject mapping, Msg-Id dedupe).
- Config `TRANSPORT` + `PUBSUB.*` (defaults + validate).
- Wiring `main.go`.
- Service `nats` (JetStream) trong `docker-compose.yml`.
- `examples/pubsub-backend/` — chạy được end-to-end.
- Tests (xem dưới).

**Ngoài scope (nêu rõ để tránh hiểu nhầm):**

- **Class-1 qua pubsub** — giữ webhook, không làm.
- **Kafka adapter** — interface đã trung lập, thêm sau không phải sửa core.
- **Chiều ngược lại (backend → client qua pubsub)**: hiện đi qua admin REST API
  (`server/restapi/mux.go:32-35`: `sendToUser`, `broadcast`, ...). Nửa `Subscribe` của
  `pubsub.Adapter` là chỗ để mở rộng sau, nhưng **không làm trong đợt này**.
- Thay `PubsubFactory` nội bộ (fanout giữa các node) — không đụng.

## Trade-offs (nêu thẳng)

- Đổi *in-memory drop* lấy *durability của broker*, nhưng **thêm một hạ tầng phải vận
  hành** (NATS). Với người dùng không cần durability, webhook vẫn là default.
- Retry/backoff chuyển từ `AsyncDispatcher` (in-process, có `DefaultBackoff`) sang
  JetStream (consumer-side). Ngữ nghĩa retry thay đổi: backend tự quyết ack/nak, thay
  vì server đẩy lại.
- Backend vẫn phải expose HTTP endpoint cho Class-1 — pubsub **không** loại bỏ hoàn
  toàn webhook. Đây là hệ quả trực tiếp của lựa chọn hybrid.

## Testing

- **Unit (`server/fns`)**: fake `AsyncTransport` in-memory — `AsyncTransport` nhỏ nên
  fake dễ. Khẳng định mỗi Class-2 hook `Emit` đúng event type + payload.
- **Regression**: `*webhook.AsyncDispatcher` vẫn thoả `AsyncTransport`; test hiện có
  của `server/webhook` giữ nguyên, không sửa.
- **`pubsubTransport`**:
  - **`Emit` không block khi broker chết** — test quan trọng nhất. Dùng adapter fake
    có `Publish` chặn; khẳng định `Emit` trả về ngay và drop-with-warning khi đầy queue.
  - Subject mapping đúng `{prefix}.{event_type}`.
  - Envelope byte-for-byte khớp `webhook.Body` (dùng chung marshalling).
  - `Msg-Id` = `Meta.CallbackID`.
- **Config**: validate `TRANSPORT=pubsub` thiếu `URL` → lỗi; defaults áp đúng.
- **Integration**: docker compose (NATS + pipewave-server + `examples/pubsub-backend`),
  kiểm end-to-end một vòng connect → message → close.

## Docker compose & example

Thêm vào `docker-compose.yml` (giữ nguyên network `pipewave`):

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

`pipewave-server` thêm `nats` vào `depends_on`. Override config qua `configs:`
`pipewave-docker-overrides` (đã có sẵn cơ chế này) đặt
`SERVER.CALLBACKS.TRANSPORT: pubsub` và `PUBSUB.URL: "nats://nats:4222"`.

Lưu ý: comment hiện có trong `docker-compose.yml` ghi rõ env override `APP_`-prefix
đang lỗi với nested field, nên phải override qua file config thứ hai — làm theo đúng
cách đó, không dùng env var.

`examples/pubsub-backend/main.go`: subscribe `pipewave.events.>`, dùng lại đúng
`switch env.Meta.EventType` như `examples/rest-backend/main.go:74`, và **vẫn phục vụ
HTTP cho Class-1** — chính điều này chứng minh hybrid chạy thật.

## Rủi ro

- **`Emit` block** là rủi ro nghiêm trọng nhất (nghẽn WebSocket hot path). Giảm thiểu
  bằng buffered channel + `select/default`, và test bắt buộc ở trên.
- **Mất event khi shutdown**: `Shutdown(ctx)` phải drain queue nội bộ best-effort
  trước khi đóng, giống `AsyncDispatcher.Shutdown`.
- **Hợp đồng durable-ack chỉ là doc comment**, compiler không ép được. Giảm thiểu bằng
  cách nêu rõ trong doc và chỉ đăng ký `natsjs` cho `DRIVER`, không cho phép trỏ
  `DRIVER` tới adapter Valkey.
