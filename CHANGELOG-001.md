# CHANGELOG — draft-001

So sánh với `origin/main`. Tag: `draft-001` (4 commits: `ebf1cd3`, `939dbd1`, `d5e3cf2`, `0fe6c31`).

---

## Tính năng mới & thay đổi lớn

### 1. Smart Container Routing (định tuyến thông điệp thông minh theo container)

Trước đây, mọi thông điệp đều được publish lên một kênh topic chung, khiến **tất cả** container nhận và xử lý rồi bỏ qua nếu không liên quan. Draft-001 thay đổi hoàn toàn mô hình này:

- Mỗi container đăng ký **hai kênh riêng biệt**:
  - Kênh riêng: `wb{containerID}` — nhận các thông điệp gửi đích danh đến container đó.
  - Kênh broadcast: `wbbc` — nhận các thông điệp gửi đến toàn bộ container (SendToAll, SendToAuthenticated, SendToAnonymous).
- Trước khi gửi, mediator service tra cứu `HolderID` (ContainerID đang giữ kết nối) từ `ActiveConnStore`, rồi chỉ publish đến đúng kênh của container đó.
- Các helper struct mới (`findUserConn`, `findSessionConn`, `findMultiUserConn` trong `99.helper.go`) đóng gói logic "tìm container → hành động" dùng chung cho toàn bộ mediator service.

**File liên quan:** `core/service/websocket/mediator/service/99.helper.go`, `core/service/websocket/broadcast/0.3.type_pubsub_msg.go`, `core/service/websocket/broadcast/1.0.msg_enum.go`

---

### 2. Cross-container ACK (xác nhận tin nhắn qua nhiều container)

`SendToSessionWithAck` và `SendToUserWithAck` nay hoạt động đúng trong môi trường multi-container:

- Khi **ContainerA** gửi `SendWithAck` mà session nằm trên **ContainerB**, ContainerA publish thông điệp `SendToSessionWithAck`/`SendToUserWithAck` kèm `SourceContainerID` vào kênh của ContainerB.
- ContainerB nhận, gửi đến client, và đăng ký `ackID → ContainerA` vào `remoteAcks` (`AckManager.RegisterRemoteAck`).
- Khi client gửi ACK về ContainerB, ContainerB kiểm tra `remoteAcks`, publish thông điệp `AckResolved` ngược lại kênh của ContainerA.
- ContainerA nhận `AckResolved`, giải phóng goroutine đang chờ (`WaitForAck`).

**File liên quan:** `core/service/websocket/ack-manager/ack_manager.go`, `core/service/websocket/broadcast-msg-handler/1_ack_resolved.go`, `core/service/websocket/broadcast-msg-handler/1_send_to_session_with_ack.go`, `core/service/websocket/broadcast-msg-handler/1_send_to_user_with_ack.go`, `core/service/websocket/client-msg-handler/0_main_handler.go`

---

### 3. Kiểu thông điệp mới trong hệ thống broadcast

Thay thế `Broadcast` (generic với `Target` enum) bằng các kiểu rõ ràng:

| Kiểu mới | Mô tả | Phạm vi |
|---|---|---|
| `SendToSessionWithAck` | Gửi đến session với ACK | Container cụ thể |
| `SendToUserWithAck` | Gửi đến user với ACK | Container cụ thể |
| `AckResolved` | Tín hiệu ACK từ container nhận về container gốc | Container cụ thể |
| `SendToAuthenticated` | Gửi đến toàn bộ kết nối đã xác thực | Tất cả container (broadcast channel) |
| `SendToAll` | Gửi đến toàn bộ kết nối | Tất cả container (broadcast channel) |

Kiểu `Broadcast` (với `BroadcastParams.Target`) đã bị **xóa**.

**File liên quan:** `core/service/websocket/broadcast/1.0.msg_enum.go`, `core/service/websocket/broadcast/1.1.params_type.go`, `core/service/websocket/broadcast/create_msg.go`

---

### 4. API thay đổi: `MsgCreator` nhận `containerIDs` làm tham số

Toàn bộ phương thức của `MsgCreator` dành cho targeted messages (SendToUser, SendToSession, DisconnectSession, DisconnectUser, SendToUsers, SendToSessionWithAck, SendToUserWithAck, AckResolved) nay nhận thêm tham số `containerIDs []string` để xác định container đích rõ ràng.

Các phương thức broadcast-to-all (SendToAnonymous, SendToAuthenticated, SendToAll) không cần `containerIDs` vì tự publish lên kênh broadcast.

---

### 5. `ActiveConnection` entity — thêm `ConnectionType`

```go
type ActiveConnection struct {
    // ...
    HolderID       string           // ContainerID đang giữ kết nối
    ConnectionType voAuth.WsCoreType // Loại kết nối: Gobwas | LongPolling
    // ...
}
```

- `HolderID` được làm rõ ngữ nghĩa: là **ContainerID** (không còn là PodName).
- Thêm field `ConnectionType` với enum `WsCoreType` (`WsCoreGobwas = 1`, `WsCoreLongPolling = 2`).
- `AddConnection` nhận thêm tham số `connectionType voAuth.WsCoreType`.

**File liên quan:** `core/domain/entities/ActiveConnection.go`, `core/domain/value-object/auth/ws_core_type.go`, `core/repository/active_conn.go`

---

### 6. Repository — phương thức mới

Thêm 2 phương thức vào interface `ActiveConnStore`:

| Phương thức | Mô tả |
|---|---|
| `GetActiveConnectionsByUserIDs(ctx, userIDs []string)` | Lấy tất cả kết nối của nhiều user cùng lúc (dùng cho SendToUsers) |
| `GetInstanceConnection(ctx, userID, instanceID string)` | Lấy thông tin kết nối của một session cụ thể (dùng để tra HolderID) |

Đã implement cho cả **DynamoDB** và **PostgreSQL**.

**File liên quan:** `core/repository/impl-dynamodb/active_conn/`, `core/repository/impl-postgres/active_conn/`

---

### 7. `WebsocketConn` interface — thêm `CoreType()`

```go
type WebsocketConn interface {
    Auth() voAuth.WebsocketAuth
    Send(payload []byte)
    CoreType() voAuth.WsCoreType  // mới
    Close()
    Ping()
}
```

`GobwasConnection` (đổi tên từ `Connection`) implement `CoreType()` trả về `WsCoreGobwas`.

**File liên quan:** `core/service/websocket/2.connection_type.go`, `core/service/websocket/server/gobwas/1_type.go`

---

### 8. `SessionInfo` — thêm trường mới

```go
type SessionInfo struct {
    UserID         string
    InstanceID     string
    HolderID       string           // mới
    ConnectionType voAuth.WsCoreType // mới
    ConnectedAt    time.Time
    IsAnonymous    bool
}
```

---

### 9. Config — thêm `ContainerID` và `HeartbeatCutoff`

```go
type globalEnvT struct {
    ContainerID     string        `koanf:"CONTAINER_ID"`     // mới
    HeartbeatCutoff time.Duration `koanf:"HEARTBEAT_CUTOFF"` // mới
    // ...
}
```

`ContainerID` được dùng xuyên suốt hệ thống routing để phân biệt container hiện tại với các container khác.

---

### 10. Sửa lỗi khởi tạo subscriber (`sync.Once`)

Trước đây dùng `map[PubsubHandler]struct{}` để tránh khởi tạo subscriber nhiều lần — không thread-safe. Nay thay bằng `sync.Once`.

---

### 11. Thêm mediator service methods mới

| Method | Mô tả |
|---|---|
| `SendToAuthenticated(ctx, msgType, payload)` | Gửi đến toàn bộ kết nối đã xác thực |
| `SendToAll(ctx, msgType, payload)` | Gửi đến toàn bộ kết nối |

---

### 12. Tài liệu thiết kế

Thêm file `docs/2026-03-25-message-hub-offline-buffering.md` mô tả kiến trúc message hub và cơ chế offline buffering.

---

## Thống kê thay đổi

- **71 file thay đổi** — 2.745 dòng thêm, 602 dòng xóa
- **File mới:** 10+ file (handler, repo impl, helper, value object, docs)
- **File xóa:** `1_broadcast.go` (mediator), `1_broadcast.go` (handler)
