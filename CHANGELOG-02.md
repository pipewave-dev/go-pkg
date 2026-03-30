# CHANGELOG-02: draft-001 → draft-002

> Kỳ phát triển: 2026-03-26 → 2026-03-30
> Tổng: 30 commits, 78 files thay đổi, +4625 / -1340 lines

---

## Tổng quan

Draft-002 tập trung hoàn thiện hai tính năng lớn:

1. **MessageHub** — cơ chế buffer tin nhắn cho session tạm ngắt kết nối (temp-disconnect)
2. **Graceful Shutdown** — quy trình tắt server an toàn, đảm bảo không mất tin nhắn khi pod bị thay thế

---

## 1. MessageHub (`core/service/websocket/msg-hub/`)

### Tính năng mới

**`MessageHubSvc` interface và implementation**

- **In-memory registry**: mỗi container lưu danh sách session đang trong trạng thái `TempDisconnected`
  - `Register(userID, instanceID, onExpired)` — đăng ký session, khởi động TTL timer
  - `Deregister(userID, instanceID)` — hủy timer, xóa khỏi registry
  - `IsRegistered(userID, instanceID)` — kiểm tra session có đang được giữ trên container này không
  - `GetSessions(userID)` — lấy danh sách instanceID của user đang temp-disconnected
- **DB persistence** (hoạt động cross-container):
  - `Save(ctx, userID, instanceID, wrappedMsg)` — lưu WebSocket frame đã wrapped vào DB
  - `Consume(ctx, userID, instanceID)` — `GetAll → DeleteAll → return`, ưu tiên duplicate hơn mất tin
- **`ShutdownSignal`** — flag để ngăn luồng temp-disconnect khi server đang graceful shutdown
- **Generation counter** — chống race condition khi timer cũ còn chạy sau khi session re-register
- **Timer leak fix** — dùng `time.NewTimer` + `defer Stop()` thay vì `time.After`

---

## 2. Pending Message Repository (`core/repository/`)

### Interface mới

```go
// core/repository/pending_message.go
type PendingMessageRepo interface {
    Create(ctx, userID, instanceID string, wrappedMsg []byte) error
    GetAll(ctx, userID, instanceID string) ([][]byte, error)
    DeleteAll(ctx, userID, instanceID string) error
}
```

### Implementations

- **DynamoDB** (`impl-dynamodb/pending_message/`): `Create`, `GetAll`, `DeleteAll`
  - DynamoDB TTL được đặt để tự cleanup record hết hạn
- **PostgreSQL** (`impl-postgres/pending_message/`): `Create`, `GetAll`, `DeleteAll`
  - Migration SQL: bảng `pending_messages` với index theo `(user_id, instance_id)`

---

## 3. WsStatus mới: `WsStatusTransferring`

### Thay đổi trong domain layer

- Thêm `WsStatusTransferring` vào `core/domain/value-object/ws/ws_status.go`
- Di chuyển `WsCoreType` từ `auth/` sang `ws/` package để tổ chức hợp lý hơn

### Repository: `UpdateStatusTransferring`

- Thêm `UpdateStatusTransferring(ctx, userID, instanceID)` vào `ActiveConnStore` interface
- **DynamoDB**: cập nhật `Status=Transferring` và xóa `HolderID` (atomic expression)
- **PostgreSQL**: tương tự, dùng `UPDATE ... SET status=transferring, holder_id=''`

---

## 4. Graceful Shutdown (`core/service/websocket/mediator/service/3.shutdown.go`)

### Luồng hoạt động mới

```
Shutdown() được gọi
  │
  ├─ 1. MarkShuttingDown()          — báo delivery layer bỏ qua DB ops trong onClose
  ├─ 2. ackManager.Shutdown()       — cancel tất cả pending ACK, unblock goroutine
  ├─ 3. Với mỗi authenticated conn:
  │     ├─ UpdateStatusTransferring()  — đặt Status=Transferring, xóa HolderID
  │     └─ MessageHub.Register()       — buffer tin nhắn mới đến trong TTL
  │           onExpired: nếu HolderID vẫn rỗng → xóa ActiveConnection
  └─ 4. Close tất cả connections    — onClose skip DB ops (đã xử lý ở bước 3)
```

**Thiết kế quan trọng**: đóng connection sau cùng để WebSocket vẫn mở trong khi DB đang update, giảm window mất tin nhắn.

---

## 5. Temp-disconnect Flow trong Delivery Layer

### `onClose` (`core/service/websocket/mediator/delivery/0.new.go`)

- **Anonymous / Shutdown** → xóa permanent như cũ
- **Authenticated + bình thường** → `UpdateStatus(TempDisconnected)` + đăng ký expiry timer vào MessageHub
- **Shutdown guard**: nếu `ShutdownSignal.IsShuttingDown()` → skip DB ops (Shutdown đã xử lý)

### `onNewRegister` (khi client reconnect)

- Phát hiện session đang ở trạng thái `TempDisconnected`:
  - **Cùng container**: gọi `MessageHub.Deregister()` trực tiếp
  - **Khác container**: publish `ResumeSession` pubsub message → container cũ hủy timer
- Sau khi resume: gọi `MessageHub.Consume()` và deliver các tin nhắn đã buffer

---

## 6. ResumeSession — Cross-container Session Recovery

### Pubsub message type mới

```go
// core/service/websocket/broadcast/1.0.msg_enum.go
WsMsgTypeResumeSession
```

### Handler trong `broadcastMsgHandler`

- Nhận `ResumeSession` event → gọi `MessageHub.Deregister()` để cancel timer trên container đang giữ session cũ
- `WsService.ResumeSession(userID, instanceID)` → publish targeted pubsub message

---

## 7. DrainableConn Interface

### Interface mới

```go
// core/service/websocket/2.connection_type.go
type DrainableConn interface {
    Drain() // flush/drain buffered messages trước khi đóng
}
```

### Implementations

- **`GobwasConnection`** (`server/gobwas/`): implement `Drain()` — flush pending frames
- **`LongPollingConn`** (`mediator/delivery/3.long_polling.go`): implement `Drain()` — flush pending responses

### Drain ordering trong `onNewRegister`

- Khi session reconnect, drain các tin nhắn đã buffer theo đúng thứ tự trước khi tiếp tục nhận tin mới

---

## 8. Routing WsStatusTransferring trong Mediator

### `findSessionConn` — thêm `transferringAction`

- Khi session ở trạng thái `Transferring`, mediator redirect sang `MessageHub.Save()` thay vì gửi trực tiếp
- `SendToSession` và `SendToSessionWithAck` đều hỗ trợ routing này

---

## 9. Wire / Dependency Injection

### `app/wire_gen.go`

- `MessageHubSvc` được tạo với `PendingMessageRepo` + TTL mặc định 5 phút
- `ShutdownSignal` được tạo và inject vào `broadcastMsgHandler`, `mediatorSvc`, `delivery`
- Tất cả các dependency mới được wire đầy đủ vào app graph

---

## 10. Config

### Thêm config cho MessageHub

- `provider/config-provider/`: thêm field cấu hình cho WebSocket MessageHub (TTL, v.v.)
- DynamoDB client: thêm hỗ trợ cấu hình bổ sung

---

## 11. Fixes

| Commit | Mô tả |
|--------|-------|
| `001ac97` | Dùng `containerID` thay `podname` cho đúng semantic |
| `6f1e672` | Xóa import `aerror` thừa trong ddb repo |
| `797c797` | Sửa panic message sai trong postgres repo stubs |
| `905e525` | Fix test dùng `range-over-int` syntax đúng chuẩn Go 1.22+ |

---

## 12. Tài liệu (docs/)

| File | Nội dung |
|------|----------|
| `specs/2026-03-27-msghub-design.md` | Design spec MessageHub |
| `plans/2026-03-27-msghub.md` | Implementation plan MessageHub |
| `specs/2026-03-30-graceful-shutdown-message-ordering-design.md` | Design spec Graceful Shutdown |
| `plans/2026-03-30-graceful-shutdown-message-ordering.md` | Implementation plan Graceful Shutdown |

---

## Kiến trúc tổng thể sau draft-002

```
Client reconnect
     │
     ▼
onNewRegister
  ├─ TempDisconnected? ──► Deregister (local) hoặc ResumeSession pubsub (remote)
  └─ Consume pending messages from DB → deliver in order

Server shutdown
     │
     ▼
Shutdown()
  ├─ MarkShuttingDown
  ├─ UpdateStatusTransferring (mỗi session)
  ├─ MessageHub.Register (buffer incoming)
  └─ Close connections

Incoming message khi session đang Transferring
     │
     ▼
findSessionConn → transferringAction → MessageHub.Save()
```
