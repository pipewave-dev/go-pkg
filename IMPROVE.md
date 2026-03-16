# Pipewave Go SDK — ModuleDelivery Review & Improvement Proposals

## 1. Tổng quan Project

Pipewave là một Go SDK cho real-time WebSocket communication, được thiết kế theo Clean Architecture với các layer rõ ràng: Delivery → Service → Repository → Domain. Project sử dụng Google Wire cho DI, gobwas/ws cho WebSocket (high-performance), Valkey/Redis cho pub/sub horizontal scaling, và hỗ trợ OpenTelemetry.

**ModuleDelivery** là interface chính mà user (developer tích hợp SDK) tương tác. Nó đóng vai trò facade, gom toàn bộ functionality của Pipewave vào một entry point duy nhất.

---

## 2. Đánh giá ModuleDelivery hiện tại

### 2.1 Các tính năng đã có

| Tính năng | Method/API | Đánh giá |
|-----------|-----------|----------|
| Gửi message đến session cụ thể | `SendToSession()` | Tốt |
| Gửi message đến tất cả session của user | `SendToUser()` | Tốt |
| Gửi message đến anonymous | `SendToAnonymous()` | Tốt |
| Kiểm tra user online | `CheckOnline()` | Cơ bản — chỉ trả bool |
| Ping kiểm tra liveness | `PingConnections()` | Tốt |
| Đăng ký callback OnNew/OnClose | `OnNewRegister()` / `OnCloseRegister()` | Tốt |
| HTTP mux với middleware | `Mux()` | Tốt |
| Monitoring (connection count, worker pool) | `Monitoring()` | Cơ bản |
| Health check | `IsHealthy()` | Cơ bản |
| Graceful shutdown | `Shutdown()` | Tốt |
| Inject custom functions | `SetFns()` | Tốt |
| Rate limiting | Config-based | Tốt |
| CORS | Config-based | Tốt |
| OpenTelemetry tracing | Config-based | Tốt |
| Message deduplication | Built-in | Tốt |
| Long-polling fallback | Transparent | Tốt |
| JSON + MessagePack serialization | Auto-detect | Tốt |

### 2.2 Điểm mạnh

- **Clean Architecture** rõ ràng, dễ mở rộng
- **Horizontal scaling** không cần sticky sessions (qua Redis pub/sub)
- **High-performance** với gobwas/ws (~0.2 KB/conn)
- **Wire DI** — compile-time safety, không có runtime reflection
- **Dual serialization** (JSON + MessagePack) — tốt cho cả web và mobile
- **Rate limiting** phân biệt user vs anonymous

### 2.3 Điểm cần cải thiện

- `CheckOnline()` chỉ trả `bool`, không có thông tin chi tiết (bao nhiêu session, device nào)
- Không có **Presence** system (ai đang online, chi tiết session)
- Thiếu **Disconnect by server** (force kick user/session)
- Thiếu **SendToMultipleUsers** (batch send)
- Thiếu **Broadcast** với target filter rõ ràng (all / auth only / anon only)
- Monitoring chưa expose đủ metrics cho production observability (Prometheus/OTEL)
- Thiếu **Connection metadata** (device type, app version, ...)
- Không có **message acknowledgment** mechanism cho phía server

---

## 3. Đề xuất tính năng mới — Mức độ ưu tiên

### 3.1 PHẢI CÓ (Critical — hầu hết real-time SDK đều cung cấp)

#### ~~3.1.1 Room/Channel Broadcasting~~ — KHÔNG đưa vào SDK

**Quyết định:** Tính năng này **không nằm trong scope** của Pipewave SDK. SDK client có thể tự implement Room/Channel bằng database riêng để quản lý membership.

**Cách client tự implement:**
- Lưu mapping `RoomID → []UserID+SessionID` vào database của application
- Khi cần broadcast tới room, query database lấy danh sách members rồi gọi `SendToUsers()` hoặc `SendToSession()`
- Việc quản lý room lifecycle (create, join, leave, delete) thuộc về business logic của application, không phải SDK

**Lý do không đưa vào SDK:**
- Room/Channel là business concept, mỗi application có model khác nhau (public room, private room, team channel, topic channel...)
- Persistence strategy khác nhau (ephemeral vs durable, TTL-based vs manual cleanup)
- Permission/authorization logic quá diverse để SDK cover
- SDK nên focus vào transport layer (gửi/nhận message), không phải application-level routing

---

#### 3.1.2 Disconnect/Kick Session từ Server

**Lý do:** Production system luôn cần khả năng force disconnect — ban user, session hijacking detection, hoặc đơn giản là admin kick.

**Đề xuất API:**

```go
type ExportedServices interface {
    // ... existing methods ...

    // Force disconnect a specific session
    DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError

    // Force disconnect all sessions of a user
    DisconnectUser(ctx context.Context, userID string) aerror.AError
}
```

**Implementation notes:**
- Broadcast qua pub/sub để tìm session trên đúng container
- Gọi `WebsocketConn.Close()` sau khi gửi close frame
- Trigger `OnCloseStuffFn` handlers bình thường

---

#### 3.1.3 SendToMultipleUsers (Batch Send)

**Lý do:** Rất phổ biến trong thực tế — gửi notification cho một nhóm users, hoặc update cho team members. Hiện tại user phải loop gọi `SendToUser()` nhiều lần, tạo overhead pub/sub không cần thiết.

**Đề xuất API:**

```go
type ExportedServices interface {
    // ... existing methods ...

    // Send to multiple users in a single broadcast
    SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError
}
```

**Implementation notes:**
- Batch pub/sub message thay vì N lần publish riêng lẻ
- Giảm đáng kể network overhead khi số lượng users lớn

---

### 3.2 NÊN CÓ (Important — nâng cao trải nghiệm developer)

#### 3.2.1 Presence System

**Lý do:** User thường cần biết ai đang online — đặc biệt trong chat, collaboration tools. Hiện `CheckOnline()` chỉ check 1 user, không có cách lấy danh sách online users hay subscribe presence changes.

**Đề xuất API:**

```go
type ExportedServices interface {
    // ... existing methods ...

    // Get online status of multiple users at once
    CheckOnlineMultiple(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError)

    // Get all currently online user IDs
    GetOnlineUsers(ctx context.Context) ([]string, aerror.AError)

    // Get detailed session info for a user
    GetUserSessions(ctx context.Context, userID string) ([]SessionInfo, aerror.AError)
}

type SessionInfo struct {
    InstanceID  string
    ConnectedAt time.Time
    IsAnonymous bool
}
```

---

#### 3.2.2 Broadcast to All (Global Broadcast)

**Lý do:** System-wide announcements, maintenance notifications, feature flag updates — rất phổ biến nhưng hiện chưa có method rõ ràng cho việc này. `SendToAnonymous(isSendAll=true)` có thể cover một phần nhưng semantic không rõ ràng và chỉ gửi cho anonymous.

**Đề xuất API:**

```go
type BroadcastTarget int

const (
    BroadcastAll       BroadcastTarget = iota // Gửi tất cả (authenticated + anonymous)
    BroadcastAuthOnly                          // Chỉ gửi cho authenticated users
    BroadcastAnonOnly                          // Chỉ gửi cho anonymous connections
)

type ExportedServices interface {
    // ... existing methods ...

    // Broadcast to connected clients based on target filter
    Broadcast(ctx context.Context, target BroadcastTarget, msgType string, payload []byte) aerror.AError
}
```

**Use cases theo target:**
- `BroadcastAll` — system maintenance notification, global announcement
- `BroadcastAuthOnly` — feature update cho logged-in users, security alert yêu cầu re-auth
- `BroadcastAnonOnly` — promotion/signup incentive cho guest users, public announcement chỉ dành cho chưa đăng nhập

**Implementation notes:**
- Reuse logic từ `SendToAnonymous` và `SendToUser` hiện có
- `BroadcastAll` = fan-out qua cả 2 kênh (user connections + anonymous connections)
- `BroadcastAuthOnly` = chỉ iterate qua authenticated connections
- `BroadcastAnonOnly` = tương tự `SendToAnonymous(isSendAll=true)` hiện tại nhưng với semantic rõ ràng hơn

---

#### 3.2.3 Connection Metadata / Custom Data

**Lý do:** User thường cần attach thêm metadata vào connection (device type, app version, user role, current page, ...) để routing message thông minh hơn.

**Đề xuất:** Mở rộng `WebsocketAuth` hoặc thêm metadata map:

```go
type ExportedServices interface {
    // ... existing methods ...

    // Set custom metadata for a connection
    SetConnectionMetadata(ctx context.Context, userID string, instanceID string, metadata map[string]string) aerror.AError

    // Get metadata of a connection
    GetConnectionMetadata(ctx context.Context, userID string, instanceID string) (map[string]string, aerror.AError)
}
```

---

### 3.3 CÓ THÌ TỐT (Nice-to-have — cạnh tranh với các SDK khác)

#### 3.3.1 Message Acknowledgment (Server-side)

**Lý do:** Cho phép server biết client đã nhận được message hay chưa — quan trọng cho chat apps, financial notifications.

**Đề xuất API:**

```go
type ExportedServices interface {
    // ... existing methods ...

    // Send with acknowledgment — returns when client confirms receipt or timeout
    SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)
}
```

---

#### 3.3.2 Metrics Export (Prometheus/OpenTelemetry Metrics)

**Lý do:** Monitoring interface hiện tại chỉ expose qua code. Production systems cần metrics endpoint cho Prometheus/Grafana. Đây là điều hầu hết real-time infrastructure đều cung cấp.

**Đề xuất:**

```go
type ModuleDelivery interface {
    // ... existing methods ...

    // Returns an http.Handler for /metrics endpoint (Prometheus format)
    MetricsHandler() http.Handler
}
```

**Metrics nên expose:**
- `pipewave_active_connections_total` (gauge, labels: type=user|anonymous)
- `pipewave_messages_sent_total` (counter, labels: target=session|user|room|broadcast)
- `pipewave_messages_received_total` (counter)
- `pipewave_connection_duration_seconds` (histogram)
- `pipewave_worker_pool_utilization` (gauge)
- `pipewave_pubsub_messages_total` (counter)

---

#### 3.3.3 Typed Message Handlers (Per-MessageType routing)

**Lý do:** Hiện tại `HandleMessage` trong `Fns` nhận tất cả message types. User phải tự switch/case. Nhiều SDK cung cấp per-type handler registration.

**Đề xuất:** Mở rộng `Fns`:

```go
type Fns struct {
    // ... existing fields ...

    // Per-type message handlers (optional, takes priority over HandleMessage)
    MessageHandlers map[string]TypedHandlerFn
}

type TypedHandlerFn func(ctx context.Context, auth WebsocketAuth, data []byte) (res []byte, err error)
```

---

## 4. So sánh với các SDK phổ biến

| Tính năng | Pipewave (hiện tại) | Socket.IO | Centrifugo | Pusher |
|-----------|---------------------|-----------|------------|--------|
| Send to user | Yes | Yes | Yes | Yes |
| Send to session | Yes | Yes | Yes | No |
| Room/Channel | Out of scope (client-side) | Yes | Yes | Yes |
| Presence | **Partial** | Yes | Yes | Yes |
| Broadcast all (with target filter) | **No** → Planned | Yes | Yes | Yes |
| Force disconnect | **No** | Yes | Yes | Yes |
| Batch send | **No** | No | Yes | Yes |
| Server-side ACK | **No** | Yes | Yes | No |
| Metrics export | **No** | Plugin | Yes | Dashboard |
| Rate limiting | Yes | No | Yes | Yes |
| Horizontal scaling | Yes | Yes (Redis) | Yes | Yes |
| Message dedup | Yes | No | Yes | No |
| Binary protocol | Yes (MsgPack) | Yes | Yes (Protobuf) | No |

---

## 5. Roadmap đề xuất

### Phase 1 — Core Missing Features
1. **DisconnectSession / DisconnectUser** — bắt buộc cho production
2. **SendToUsers (batch)** — giảm overhead, cải thiện DX
3. **Broadcast (global với target filter)** — hỗ trợ All / AuthOnly / AnonOnly

### Phase 2 — Enhanced Observability & Presence
4. **Presence system** (CheckOnlineMultiple, GetOnlineUsers, GetUserSessions)
5. **Prometheus/OTEL metrics export**
6. **Connection metadata**

### Phase 3 — Advanced Features
7. **Message acknowledgment**
8. **Typed message handlers**

### Không đưa vào SDK (client tự implement)
- **Room/Channel Broadcasting** — client sử dụng database riêng để quản lý membership (RoomID → UserID+SessionID), kết hợp với `SendToUsers()` hoặc `SendToSessions()` để broadcast

---

## 6. Kết luận

ModuleDelivery hiện tại cung cấp một **nền tảng tốt** cho 1-to-1 messaging và basic monitoring. SDK tập trung vào **transport layer** — gửi/nhận message hiệu quả với horizontal scaling.

**Triết lý thiết kế:** Pipewave SDK focus vào việc làm tốt transport layer, không bao gồm application-level concepts như Room/Channel (client tự implement với database riêng, kết hợp với `SendToUsers()` hoặc `SendToSessions()` để broadcast.

**Ưu tiên cao nhất:** DisconnectSession/DisconnectUser, SendToUsers (batch), và Broadcast với target filter. Ba nhóm tính năng này sẽ giúp Pipewave cover được phần lớn use cases production (force close, batch notification, system announcement) và cung cấp building blocks đủ mạnh để client tự build Room/Channel logic phía trên.

Infrastructure hiện có (Redis pub/sub, ConnectionManager, horizontal scaling) đã sẵn sàng để hỗ trợ các tính năng mới này mà không cần thay đổi kiến trúc lớn.
