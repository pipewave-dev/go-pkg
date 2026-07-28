# Metrics Observability — Prometheus Instrumentation cho Connection, Message & Callback

**Date:** 2026-07-28
**Status:** Approved

---

## Overview

pipewave hiện có tracing (`provider/otel-provider`) và một REST monitoring API
(`GET /api/v1/monitoring/connections`, `/worker-pool`) nhưng **không có metrics
time-series**. Không có metrics nghĩa là không alert được, không thấy trend, và
không trả lời được "tuần trước có chậm hơn không".

`pkg/metrics/metrics.go` đã tồn tại với 5 instrument OTEL nhưng là **scaffolding
chết**: `metrics.New()` được gọi ở `core/delivery/module/0.0.new.go:31` và gán
vào struct, nhưng **không có một lệnh `Record*` nào được gọi** ở bất kỳ đâu trong
codebase, và **không có endpoint `/metrics`** nào được register. Nền tảng (OTEL
SDK + Prometheus exporter) là đúng — chỉ thiếu wiring, và có một lỗi kiến trúc
phải sửa (xem Architecture).

Thiết kế này bổ sung ba nhóm năng lực:

1. **Metrics transport** — `provider/metrics-provider` mới: Prometheus exporter +
   MeterProvider + listener riêng `:9090` phục vụ `/metrics`.
2. **Tier 1 instruments** — connection lifecycle (accept/reject/duration/active)
   và inbound client message (count/latency/outcome).
3. **Tier 2 instruments** — webhook callback duration/errors/retries/dropped +
   circuit-breaker state. Đây là nhóm giá trị cao nhất *riêng cho kiến trúc
   container* của pipewave: backend của khách hàng nằm trên sync fail-closed path
   của `inspect_token` / `on_new_connection` / `handle_message`, nên **backend
   chậm = không ai connect được**.

**Nguyên tắc isolation cốt lõi:** `pkg/metrics` chỉ tạo instrument từ
`otel.GetMeterProvider()`. Nó **không** tạo MeterProvider, **không** mở port,
**không** biết Prometheus tồn tại. Và `server/webhook` **không** import
`pkg/metrics` — nó chỉ nhận một optional observer hook. Điều này giữ cả hai
package testable thuần túy, đúng tinh thần spec callback-resilience ("package
`webhook` KHÔNG biết gì về HTTP server hay `os.Exit`").

---

## Architecture

Vấn đề phải sửa: pipewave vừa là **library nhúng** (`pipewave.New()`) vừa là
**container** (`cmd/pipewave-server`). Code hiện tại gọi `prometheus.New()` +
`otel.SetMeterProvider()` ngay trong constructor (`pkg/metrics/metrics.go:24-30`),
nghĩa là library **ghi đè global MeterProvider của host app**. Đó là lỗi kiến
trúc, không chỉ là thiếu wiring.

```
                    ┌─────────────────────────────────────────┐
                    │ otel.GetMeterProvider()  (global)       │
                    └───────────────▲─────────────┬───────────┘
      container: đặt provider vào   │             │  library: chỉ đọc ra
                                    │             ▼
  provider/metrics-provider ────────┘      pkg/metrics.New()
  - prometheus exporter                    - tạo instrument
  - MeterProvider                          - no-op nếu host chưa set provider
  - http.Server :9090 /metrics
  - RegTask(shutdown)
```

Hệ quả quan trọng: Go embedder không set provider → `pkg/metrics` nhận no-op
meter, **zero overhead, không cần config gì**. Embedder muốn có metrics thì tự
set global MeterProvider của họ (hoặc dùng `metrics-provider` này) — instrument
tự động chảy vào registry của họ, không xung đột.

| Unit | Trách nhiệm | File |
|---|---|---|
| **Config** | `METRICS.ENABLED/PORT/PATH/MSG_TYPE_ALLOWLIST` | `export/types/config_child.go` |
| **metrics-provider** | Prometheus exporter + MeterProvider + listener `:9090` | `provider/metrics-provider/metrics.go` (mới) |
| **Instruments (core)** | Tier 1: connection, message | `pkg/metrics/metrics.go` (viết lại) |
| **Instruments (callback)** | Tier 2: callback duration/errors/retries | `pkg/metrics/callback.go` (mới) |
| **Gauge callbacks** | ObservableGauge đọc từ `business.Monitoring` | `pkg/metrics/observable.go` (mới) |
| **Sanitize** | `msg_type` byte→tên, allowlist, `reason` classify | `pkg/metrics/sanitize.go` (mới) |
| **Wiring container** | Đăng ký provider vào DI graph + listener | `do_packages.go`, `cmd/pipewave-server/main.go` |

### Transport & endpoint

- **Prometheus scrape**, không push OTLP. Staging đang set `OTEL.EXPORTER_TYPE:
  discard` nên chưa có collector chạy thật; team đã có Grafana
  (`grafana.k8s.ponos-tech.com`).
- **Listener riêng `:9090`**, tách hoàn toàn khỏi admin API `:8081` và WS `:8080`.
  Không yêu cầu API key — port này không expose ra internet. Tách riêng giúp
  network policy sạch: không phải mở admin API cho Prometheus.

---

## 1. Config

Thêm `MetricsT` vào `export/types/config_child.go`, theo đúng pattern `OtelT`
hiện có. **Mọi field đều có default = hành vi hiện tại** (không metrics) —
backward compatible.

```yaml
METRICS:
  ENABLED: false          # opt-in
  PORT: 9090
  PATH: "/metrics"
  MSG_TYPE_ALLOWLIST: []  # rỗng ⇒ app msg_type gộp thành "other"
```

`validate()` — fail fast lúc boot, không phải lúc runtime:

- `ENABLED && PORT <= 0 || PORT > 65535` → panic.
- `ENABLED && PATH` không bắt đầu bằng `/` → panic.
- Mỗi entry trong `MSG_TYPE_ALLOWLIST`: không printable ASCII hoặc `len > 32` →
  panic.

`loadDefault()` — `PORT = 9090`, `PATH = "/metrics"` khi rỗng.

---

## 2. Tier 1 — Connection & Message (8 metric)

| Metric | Type | Labels |
|---|---|---|
| `pipewave_connections_active` | ObservableGauge | `auth`=anon\|user |
| `pipewave_users_active` | ObservableGauge | — |
| `pipewave_connections_accepted_total` | Counter | `transport`=ws\|longpoll, `auth` |
| `pipewave_connections_rejected_total` | Counter | `transport`, `reason` |
| `pipewave_connection_duration_seconds` | Histogram | `auth` |
| `pipewave_client_messages_total` | Counter | `msg_type`, `outcome` |
| `pipewave_client_message_duration_seconds` | Histogram | `msg_type` |
| `pipewave_build_info` | Gauge=1 | `version`, `container_id` |

`build_info` lấy `version` từ `global/constants.Version` và `container_id` từ
`Info.ContainerID` (`export/types/config_child.go:14`, auto-generate nếu rỗng).
`container_id` chỉ xuất hiện ở metric này — **không bao giờ** làm label trên
counter/histogram.

`transport` có đúng hai giá trị: `ws` (`GobwasEndpoint`) và `longpoll`
(`mediator/delivery/3.long_polling.go`).

### Gauge phải là ObservableGauge, không phải UpDownCounter

Code hiện tại dùng `Int64UpDownCounter` cho `activeConnections`. Sai: chỉ cần
một lần miss `RecordConnectionClose` (panic, crash, ctx cancel) là counter
**drift vĩnh viễn** và không bao giờ tự sửa.

Thay bằng `Int64ObservableGauge` với callback đọc từ `business.Monitoring`
(`core/service/business/monitoring.go`) — interface này đã trả về đúng dữ liệu
cần: `InsideActiveConnection()` → `AnonymosConnection`, `UserConnection`,
`TotalUser`. Mỗi lần scrape đọc state thật, không tích lũy sai số.

`pipewave_connections_active` dùng `InsideActiveConnection()` (số trong container
này) chứ không phải `TotalActiveConnection()` — mỗi pod report phần của mình,
Prometheus `sum()` lên là ra tổng. Dùng total sẽ bị đếm trùng N lần với N pod.

### Label `reason` cho rejected

Tập đóng, lấy đúng từ các nhánh return trong
`core/service/websocket/mediator/delivery/2.gobwas_endpoint.go:22-60`:

| `reason` | Nguồn |
|---|---|
| `missing_token` | `connToken == ""` (line 24) |
| `invalid_token` | `ScanConnToken` lỗi (line 31) |
| `upgrade_failed` | `ws.UpgradeHTTP` lỗi (line 40) |
| `register_failed` | `NewConnection` / `onNewStuff.Do` lỗi (line 48, 58) |
| `rate_limited` | ip-rate-limiter (`mediator/delivery/ip_rate_limiter.go`) |

### Label `outcome` cho client message

Tập đóng: `ok`, `error`, `invalid_schema`, `dedup`, `throttled`, `rate_limited`.
Lấy từ các nhánh trong `core/service/websocket/client-msg-handler/0_main_handler.go`.

### `msg_type` — sanitize bắt buộc

`MessageType` trong `core/service/websocket/0.message_type.go:7-12` là
**một byte đơn không in được** cho system types, không phải string dễ đọc:

```go
MessageTypeHeartbeat = MessageType([]byte{202})
MessageTypeAck       = MessageType([]byte{203})
```

Dùng raw byte làm label sẽ ra ký tự rác trong Grafana. Và app-level `msg_type`
đến từ msgpack **do client kiểm soát** — không lọc thì một client lỗi (hoặc cố
ý) làm nổ cardinality Prometheus.

`sanitizeMsgType(MessageType) string`:

1. Byte `202` → `"heartbeat"`, `203` → `"ack"` (map system types sang tên có nghĩa).
2. Ngược lại: giữ nguyên **chỉ khi** có trong `MSG_TYPE_ALLOWLIST` **và** là
   printable ASCII **và** `len <= 32`.
3. Mọi trường hợp khác → `"other"`.

Default allowlist rỗng ⇒ mặc định mọi app message gộp vào `"other"`; khách hàng
tự khai báo type nào muốn theo dõi riêng. An toàn theo mặc định.

---

## 3. Tier 2 — Webhook Callbacks (5 metric)

| Metric | Type | Labels |
|---|---|---|
| `pipewave_callback_duration_seconds` | Histogram | `event_type`, `mode`=sync\|async |
| `pipewave_callback_errors_total` | Counter | `event_type`, `mode`, `reason` |
| `pipewave_callback_retries_total` | Counter | `event_type`, `mode` |
| `pipewave_callback_dropped_total` | Counter | `event_type` |
| `pipewave_callback_breaker_open` | ObservableGauge | — (0/1) |

`event_type` là tập đóng 9 giá trị từ callback contract (`README.md`):
`inspect_token`, `handle_message`, `on_new_connection`,
`on_new_connection_established`, `on_close_connection`, `on_read_error`,
`on_write_error`, `message_received`, `ping`. → an toàn cardinality.

### Label `reason` — tách timeout khỏi 5xx

`classifyCallbackError(error, statusCode) string`:

| `reason` | Điều kiện |
|---|---|
| `timeout` | `errors.Is(err, context.DeadlineExceeded)` hoặc `net.Error.Timeout()` |
| `conn_refused` | `syscall.ECONNREFUSED` |
| `dns` | `*net.DNSError` |
| `status_4xx` | `400 <= code < 500` |
| `status_5xx` | `code >= 500` |
| `bad_body` | JSON unmarshal / schema lỗi |
| `breaker_open` | `CircuitBreaker.Allow() == false` |
| `other` | fallback |

Tách `timeout` khỏi `status_5xx` là thiết yếu — hai cái này có nguyên nhân gốc và
cách xử lý hoàn toàn khác nhau, và sẽ là câu hỏi số một khi on-call.

### Một điểm nối duy nhất

`Sender.Post` (`server/webhook/sender.go:32`) là chỗ **mọi** callback đi qua —
cả sync (`sync.go`) và async (`async.go`). Nên chỉ cần một hook ở đây, không rải
code metrics khắp hai file.

`server/webhook` **không import `pkg/metrics`**. Thay vào đó `webhook` định nghĩa
một interface tối thiểu và nhận nil-able implementation:

```go
// server/webhook/sender.go
type CallObserver interface {
    ObserveCall(eventType, mode string, dur time.Duration, err error, statusCode int)
}
```

`Sender` gọi `if s.obs != nil { s.obs.ObserveCall(...) }`. `pkg/metrics` cung cấp
một type thoả interface đó; `cmd/pipewave-server/main.go` là nơi duy nhất nối hai
bên. `webhook` giữ nguyên tính testable thuần túy (test hiện có không cần sửa —
observer là optional, nil trong test).

`mode` được `Sender` nhận từ caller (sync/async) vì `Post` không tự biết nó đang
được gọi từ đường nào.

`retries_total` đếm ở `SyncCaller.Call` (`sync.go:98`) và
`AsyncDispatcher.deliver` (`async.go:108`) — mỗi attempt thứ 2 trở đi tăng 1.
`dropped_total` đếm khi async retry exhausted.

`breaker_open` dùng `CircuitBreaker.OpenSince()` đã có sẵn (`sync.go:69`) — không
cần thêm state mới.

---

## Error Handling

Metrics **tuyệt đối không được làm chết đường chính**. Đây là ràng buộc thiết kế,
không phải best-effort:

| Tình huống | Xử lý |
|---|---|
| `meter.Int64Counter()` trả error | Log warn + dùng no-op instrument. **Không panic** (code hiện tại panic ở `metrics.go:26` — sẽ bỏ) |
| Listener `:9090` fail bind | Log error, server chính vẫn chạy bình thường |
| Global MeterProvider chưa set | Nhận no-op meter, mọi `Record*` là no-op |
| ObservableGauge callback lỗi/chậm | Bọc timeout 2s, trả giá trị cached lần trước. Callback gọi `Monitoring` có DB access — không được để scrape treo |
| `sanitizeMsgType` gặp input lạ | Trả `"other"`, không bao giờ error |

ObservableGauge timeout là điểm dễ bỏ sót nhất: `TotalActiveConnection()` query
DynamoDB/Postgres. Nếu DB chậm và Prometheus scrape mỗi 15s, callback không có
timeout sẽ làm scrape pile up.

---

## Testing

Ưu tiên test phần logic thuần — đó là nơi bug thật sự nằm:

- **`sanitizeMsgType`** — system bytes (`202`/`203`), app type trong allowlist,
  ngoài allowlist, non-printable, `len > 32`, empty. Table-driven.
- **`classifyCallbackError`** — mỗi `reason` một case, gồm wrapped error
  (`fmt.Errorf("%w")`) để đảm bảo dùng `errors.Is`/`errors.As` chứ không so sánh
  string.
- **Instruments** — `sdkmetric.NewManualReader` để assert metric được record với
  đúng name + label, không cần HTTP. Đây là cách test OTEL instrument chuẩn.
- **ObservableGauge** — inject `Monitoring` stub trả lỗi/treo, assert callback trả
  cached value và không block quá timeout.
- **Listener** — chạy thật trên port `0`, GET `/metrics`, assert body chứa metric
  name. Một test đủ.
- **No-op path** — assert `metrics.New()` khi global provider chưa set thì không
  panic và `Record*` an toàn.

`server/webhook` test hiện có **không cần sửa** — observer là optional/nil.
Thêm một test mới: `Sender` với observer stub, assert `ObserveCall` được gọi
đúng `mode`/`event_type`/`statusCode`.

---

## Out of Scope

Ghi lại làm follow-up, không làm lần này:

- **k8s manifest** — ServiceMonitor hoặc `prometheus.io/scrape` annotation trên
  `k8s/base-app/03-service.yaml`. Nằm ở repo khác (`k8s/`), làm riêng sau khi
  endpoint đã chạy.
- **Grafana dashboard JSON** — làm sau khi có data thật để biết panel nào hữu ích.
- **Tier 3** — broadcast, ACK pending, pubsub lag, pending messages. Đáng làm,
  nhưng phải sửa nhiều file trong `core/service/websocket` nên tách PR riêng.
  Riêng `pipewave_ack_pending` (gauge đọc `len(AckManager.pending)`) nên ưu tiên
  ở PR kế tiếp — nó phát hiện leak trước khi thành OOM.
- **Tier 4** — worker pool depth/dropped, repo latency, rate-limit hits.
  `workerpool.Stat().DroppedTasks` đã có sẵn nên rất rẻ; đưa vào PR kế tiếp.
- **End-to-end delivery latency** — `pipewave_delivery_latency_seconds` từ lúc
  REST nhận `POST /messages/user` đến lúc frame ra socket (đo qua timestamp trong
  `pubsubMessage.otelCarrier`). Đây là con số duy nhất khách hàng thực sự quan
  tâm, nhưng đắt để implement đúng — cần spec riêng.
