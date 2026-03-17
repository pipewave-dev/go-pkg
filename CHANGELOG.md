# Changelog

## [Unreleased] — 2026-03-16

### Commit: `a641844` — feat: implement 7 websocket platform features from IMPROVE.md

Commit này bổ sung **7 tính năng lớn** cho Pipewave Go SDK, nâng cấp khả năng quản lý connection, broadcasting, presence, metrics và message acknowledgment. Tất cả tính năng đều tuân theo Clean Architecture hiện có (Delivery → Service → Repository → Domain) và hỗ trợ horizontal scaling qua pub/sub.

---

## 1. DisconnectSession / DisconnectUser — Force Disconnect từ Server

### Mô tả
Cho phép server chủ động ngắt kết nối WebSocket của một session cụ thể hoặc toàn bộ session của một user. Trước đây SDK **không có khả năng** force disconnect — đây là tính năng bắt buộc cho production (ban user, phát hiện session hijacking, admin kick).

### API mới

```go
// Force disconnect một session cụ thể (ví dụ: kick 1 tab/device)
svc.DisconnectSession(ctx, "user-123", "instance-abc")

// Force disconnect TẤT CẢ session của user (ví dụ: ban user)
svc.DisconnectUser(ctx, "user-123")
```

### Cách hoạt động
- Gửi message qua pub/sub channel (`DisconnectSession` / `DisconnectUser`) để tìm đúng container đang giữ connection.
- Container nhận được message → tìm connection trong `ConnectionManager` → gọi `conn.Close()`.
- Callback `OnCloseStuffFn` vẫn được trigger bình thường sau khi close.

### Files mới
| File | Mô tả |
|------|--------|
| `core/service/websocket/broadcast-msg-handler/1_disconnect_session.go` | Handler xử lý pub/sub message DisconnectSession trên mỗi container |
| `core/service/websocket/broadcast-msg-handler/1_disconnect_user.go` | Handler xử lý pub/sub message DisconnectUser trên mỗi container |
| `core/service/websocket/mediator/service/1.disconnect_session.go` | Service publish message DisconnectSession qua pub/sub |
| `core/service/websocket/mediator/service/1.disconnect_user.go` | Service publish message DisconnectUser qua pub/sub |

### Files thay đổi
| File | Thay đổi |
|------|----------|
| `core/delivery/module.go` | Thêm `DisconnectSession()`, `DisconnectUser()` vào interface `ExportedServices` |
| `core/service/websocket/0.ws_service.go` | Thêm vào interface `WsService` |
| `core/service/websocket/broadcast/1.0.msg_enum.go` | Thêm pub/sub channel `channelDisconnectSession`, `channelDisconnectUser` |
| `core/service/websocket/broadcast/1.1.params_type.go` | Thêm `DisconnectSessionParams`, `DisconnectUserParams` (msgpack serialization) |
| `core/service/websocket/broadcast/create_msg.go` | Thêm `MsgCreator.DisconnectSession()`, `MsgCreator.DisconnectUser()` + subscriber registration |
| `core/delivery/module/3.get_service.go` | Wire delegate methods |

### Chú ý
- DisconnectSession yêu cầu cả `userID` + `instanceID` (session ID).
- DisconnectUser sẽ ngắt **tất cả** session, bao gồm multi-tab và multi-device.
- Connection close sẽ trigger `OnCloseStuffFn` handler đã đăng ký — đảm bảo cleanup logic vẫn chạy đúng.

---

## 2. SendToUsers — Batch Send tới nhiều Users

### Mô tả
Gửi message tới **nhiều users cùng lúc** chỉ với 1 lần publish qua pub/sub. Trước đây developer phải loop gọi `SendToUser()` N lần, tạo N message pub/sub riêng lẻ.

### API mới

```go
// Gửi notification cho nhiều users cùng lúc
userIDs := []string{"user-1", "user-2", "user-3"}
svc.SendToUsers(ctx, userIDs, "team_update", payload)
```

### Cách hoạt động
- Publish **1 message** pub/sub chứa danh sách `userIDs`.
- Mỗi container nhận message → iterate qua `userIDs` → tìm connections local → send.
- Giảm đáng kể network overhead so với N lần publish riêng lẻ.

### Files mới
| File | Mô tả |
|------|--------|
| `core/service/websocket/broadcast-msg-handler/1_send_to_users.go` | Handler fan-out message tới connections của nhiều users |
| `core/service/websocket/mediator/service/1.send_notification_to_users.go` | Service publish SendToUsers qua pub/sub |

### Files thay đổi
| File | Thay đổi |
|------|----------|
| `core/delivery/module.go` | Thêm `SendToUsers()` vào `ExportedServices` |
| `core/service/websocket/0.ws_service.go` | Thêm vào `WsService` interface |
| `core/service/websocket/broadcast/1.0.msg_enum.go` | Thêm `channelSendToUsers` |
| `core/service/websocket/broadcast/1.1.params_type.go` | Thêm `SendToUsersParams` struct |
| `core/service/websocket/broadcast/create_msg.go` | Thêm `MsgCreator.SendToUsers()` + subscriber |
| `core/delivery/module/3.get_service.go` | Wire delegate |

### Chú ý
- Payload được serialize **1 lần** và gửi cho tất cả users — hiệu quả hơn nhiều so với loop `SendToUser()`.
- Mỗi user nhận message trên **tất cả sessions** (multi-tab, multi-device), giống behavior của `SendToUser()`.

---

## 3. Broadcast — Global Broadcast với Target Filter

### Mô tả
Broadcast message tới **tất cả connections** đang kết nối, với khả năng lọc theo loại connection. Hỗ trợ 3 target:

| Target | Mô tả | Use case |
|--------|--------|----------|
| `BroadcastAll` | Tất cả connections (authenticated + anonymous) | System maintenance, global announcement |
| `BroadcastAuthOnly` | Chỉ authenticated users | Feature update, security alert yêu cầu re-auth |
| `BroadcastAnonOnly` | Chỉ anonymous connections | Promotion cho guest, signup incentive |

### API mới

```go
import "github.com/pipewave-dev/go-pkg/core/delivery"

// Broadcast tới tất cả
svc.Broadcast(ctx, delivery.BroadcastAll, "maintenance", payload)

// Chỉ authenticated users
svc.Broadcast(ctx, delivery.BroadcastAuthOnly, "feature_update", payload)

// Chỉ anonymous
svc.Broadcast(ctx, delivery.BroadcastAnonOnly, "signup_promo", payload)
```

### Cách hoạt động
- Publish 1 message pub/sub chứa `target` (int enum).
- Mỗi container nhận message → dựa vào `target` để chọn nhóm connections:
  - `BroadcastAll` → `GetAllConnections()`
  - `BroadcastAuthOnly` → `GetAllAuthenticatedConn()` **(method mới)**
  - `BroadcastAnonOnly` → `GetAllAnonymousConn()`

### Files mới
| File | Mô tả |
|------|--------|
| `core/service/websocket/broadcast-msg-handler/1_broadcast.go` | Handler fan-out theo target filter |
| `core/service/websocket/mediator/service/1.broadcast.go` | Service publish Broadcast qua pub/sub |

### Files thay đổi
| File | Thay đổi |
|------|----------|
| `core/delivery/module.go` | Thêm type `BroadcastTarget` + constants, thêm `Broadcast()` vào `ExportedServices` |
| `core/service/websocket/3_connection_manager.go` | Thêm `GetAllAuthenticatedConn()` vào interface `ConnectionManager` |
| `core/service/websocket/connection-manager/connection_mamanger.go` | Implement `GetAllAuthenticatedConn()` — iterate `userConn` map |
| `core/service/websocket/broadcast/1.0.msg_enum.go` | Thêm `channelBroadcast` |
| `core/service/websocket/broadcast/1.1.params_type.go` | Thêm `BroadcastParams` |

### Chú ý
- `BroadcastTarget` là `int` enum defined trong `core/delivery/module.go`. Import package `delivery` để sử dụng constants.
- Broadcast đi qua pub/sub nên hoạt động đúng trên **multi-container** deployment.
- `GetAllAuthenticatedConn()` là method mới trên `ConnectionManager` — iterate qua map `userConn` (thread-safe với RWMutex).

---

## 4. Presence System — CheckOnlineMultiple + GetUserSessions

### Mô tả
Hệ thống Presence cho phép kiểm tra online status của **nhiều users cùng lúc** và lấy **chi tiết sessions** của một user. Trước đây chỉ có `CheckOnline()` kiểm tra 1 user, trả về `bool` đơn giản.

### API mới

```go
// Kiểm tra online status nhiều users cùng lúc
statuses, err := svc.CheckOnlineMultiple(ctx, []string{"user-1", "user-2", "user-3"})
// statuses = map[string]bool{"user-1": true, "user-2": false, "user-3": true}

// Lấy chi tiết sessions của 1 user
sessions, err := svc.GetUserSessions(ctx, "user-1")
// sessions = []SessionInfo{
//   {InstanceID: "tab-1", ConnectedAt: time.Time{...}, IsAnonymous: false},
//   {InstanceID: "mobile-1", ConnectedAt: time.Time{...}, IsAnonymous: false},
// }
```

### SessionInfo struct

```go
type SessionInfo struct {
    InstanceID  string    // Session/instance ID
    ConnectedAt time.Time // Thời điểm kết nối
    IsAnonymous bool      // Có phải anonymous không
}
```

### Cách hoạt động
- **CheckOnlineMultiple**: Query database `CountActiveConnectionsBatch()` — đếm connections có `LastHeartbeat` trong 2 phút gần nhất.
  - DynamoDB: Loop query từng userID (partition key).
  - PostgreSQL: Dùng `WHERE user_id = ANY($1)` + `GROUP BY` cho 1 query duy nhất.
- **GetUserSessions**: Query database `GetActiveConnections()` — trả về tất cả active connections của user.

### Files mới
| File | Mô tả |
|------|--------|
| `core/service/websocket/mediator/service/1.check_online_multiple.go` | Service CheckOnlineMultiple |
| `core/service/websocket/mediator/service/1.get_user_sessions.go` | Service GetUserSessions |
| `core/repository/impl-dynamodb/active_conn/count_active_connections_batch.go` | DynamoDB impl — loop query đếm connections |
| `core/repository/impl-dynamodb/active_conn/get_active_connections.go` | DynamoDB impl — query all connections by userID |
| `core/repository/impl-dynamodb/active_conn/exprbuilder/0.4.querier.go` | Thêm `QueryByUserID()` + `CountActive()` cho DynamoDB expression builder |
| `core/repository/impl-postgres/active_conn/count_active_connections_batch.go` | PostgreSQL impl — single query với `ANY($1)` |
| `core/repository/impl-postgres/active_conn/get_active_connections.go` | PostgreSQL impl — query connections by userID |

### Files thay đổi
| File | Thay đổi |
|------|----------|
| `core/delivery/module.go` | Thêm `SessionInfo` type alias, `CheckOnlineMultiple()`, `GetUserSessions()` vào `ExportedServices` |
| `core/service/websocket/0.ws_service.go` | Thêm `SessionInfo` struct + methods vào `WsService` |
| `core/repository/active_conn.go` | Thêm `CountActiveConnectionsBatch()`, `GetActiveConnections()` vào `ActiveConnStore` interface |
| `core/domain/entities/ActiveConnection.go` | **Thêm field `ConnectedAt time.Time`** |
| `core/repository/impl-dynamodb/active_conn/exprbuilder/1.ddb.go` | Thêm `FieldConnectedAt`, `ConnectedAt` trong DDB mapping |
| `core/repository/impl-dynamodb/active_conn/exprbuilder/0.2.creator.go` | Set `ConnectedAt = now` khi tạo connection |

### Chú ý
- **Breaking change trên entity**: `ActiveConnection` có thêm field `ConnectedAt`. Các connections tạo trước khi deploy version mới sẽ có `ConnectedAt = zero time`. Không ảnh hưởng tới logic hiện tại vì field này chỉ được đọc bởi `GetUserSessions()`.
- Cutoff heartbeat là **2 phút** (hardcoded) — tương ứng với `GlobalHeartbeatRateDuration`.
- PostgreSQL implementation hiệu quả hơn DynamoDB cho batch check (1 query vs N queries).

---

## 5. Prometheus / OpenTelemetry Metrics Export

### Mô tả
Thêm `/metrics` endpoint trả về metrics ở format Prometheus. Dùng **OpenTelemetry SDK** + **Prometheus exporter** — tương thích với cả OTEL collector và Prometheus scraper.

### API mới

```go
// Mount metrics endpoint
mux := http.NewServeMux()
mux.Handle("/metrics", pw.MetricsHandler())
```

### Metrics instruments

| Metric | Type | Labels | Mô tả |
|--------|------|--------|--------|
| `pipewave_active_connections` | UpDownCounter (Gauge) | `type` (user/anonymous) | Số connections đang active |
| `pipewave_messages_sent_total` | Counter | `target` (session/user/broadcast) | Tổng messages đã gửi |
| `pipewave_messages_received_total` | Counter | — | Tổng messages nhận từ client |
| `pipewave_connection_duration_seconds` | Histogram | `type` (user/anonymous) | Thời gian kết nối |
| `pipewave_pubsub_messages_total` | Counter | — | Tổng pub/sub messages published |

### Files mới
| File | Mô tả |
|------|--------|
| `pkg/metrics/metrics.go` | `PipewaveMetrics` struct — tạo OTEL meter + Prometheus exporter, expose `Handler()` và các method `Record*()` |

### Files thay đổi
| File | Thay đổi |
|------|----------|
| `core/delivery/module.go` | Thêm `MetricsHandler() http.Handler` vào `ModuleDelivery` interface |
| `core/delivery/module/0.0.new.go` | Thêm field `metrics *metrics.PipewaveMetrics`, khởi tạo trong `New()`, implement `MetricsHandler()` |

### Dependencies mới (go.mod)
- `github.com/prometheus/client_golang` — Prometheus client
- `go.opentelemetry.io/otel/exporters/prometheus` — OTEL Prometheus exporter
- `go.opentelemetry.io/otel/sdk/metric` — OTEL metrics SDK

### Chú ý
- Metrics hiện được **tạo** nhưng các method `Record*()` chưa được gọi từ các handler hiện tại. Cần tích hợp thêm trong connection handler và message handler để metrics có data thực tế.
- `MetricsHandler()` được thêm vào `ModuleDelivery` interface — **breaking change** nếu có code implement interface này bên ngoài SDK.
- Prometheus exporter tự động register với global OTEL `MeterProvider`.

---

## 6. Connection Metadata

### Mô tả
Cho phép attach **custom metadata** (device type, app version, user role, ...) vào mỗi connection ngay từ lúc authenticate. Metadata được truyền từ `InspectToken` function.

### Thay đổi API — **BREAKING CHANGE** (2 thay đổi)

**Trước (cũ):**
```go
type Fns struct {
    InspectToken func(ctx context.Context, token string) (username string, IsAnonymous bool, err error)
}
```

**Sau (mới):**
```go
type Fns struct {
    InspectToken func(ctx context.Context, token string, headers http.Header) (username string, IsAnonymous bool, metadata map[string]string, err error)
}
```

### Ví dụ sử dụng

```go
pw.SetFns(&configprovider.Fns{
    InspectToken: func(ctx context.Context, token string, headers http.Header) (string, bool, map[string]string, error) {
        userID, err := validateJWT(ctx, token)
        if err != nil {
            return "", false, nil, err
        }
        metadata := map[string]string{
            "device":      headers.Get("X-Device-Type"),  // đọc từ HTTP headers
            "app_version": headers.Get("X-App-Version"),
        }
        return userID, false, metadata, nil
    },
})
```

### WebsocketAuth struct thay đổi

```go
type WebsocketAuth struct {
    UserID     string
    InstanceID string
    Metadata   map[string]string  // MỚI
}
```

### Constructors mới

```go
// Tạo auth với metadata
voAuth.UserWebsocketAuthWithMetadata(userID, instanceID, metadata)
voAuth.AnonymousUserWebsocketAuthWithMetadata(instanceID, metadata)
```

### Files thay đổi
| File | Thay đổi |
|------|----------|
| `provider/config-provider/0.4.fns.go` | **BREAKING**: `InspectToken` signature thêm param `headers http.Header` và return value `metadata map[string]string` |
| `core/domain/value-object/auth/ws_auth.go` | Thêm field `Metadata`, thêm 2 constructor mới `*WithMetadata()` |
| `core/service/websocket/mediator/delivery/1.issue_tmp_token.go` | Truyền `r.Header` vào `InspectToken`, nhận `metadata`, dùng `*WithMetadata()` constructors |
| `core/service/websocket/mediator/delivery/3.long_polling.go` | Tương tự — truyền `r.Header` + nhận `metadata` cho long-polling endpoint |
| `core/service/websocket/mediator/delivery/4.long_polling_send.go` | Tương tự — truyền `r.Header` + nhận `metadata` cho long-polling send endpoint |

### Chú ý — **BREAKING CHANGE cho người dùng SDK**
- **Tất cả code implement `InspectToken`** phải cập nhật signature:
  1. Thêm param `headers http.Header` (param thứ 3)
  2. Thêm return value `metadata map[string]string` (return thứ 3)
- `headers` chứa toàn bộ HTTP headers của request gốc (`r.Header`) — cho phép đọc custom headers (device type, app version, ...) ngay trong `InspectToken` mà không cần thêm middleware.
- Nếu không cần metadata và headers, vẫn phải nhận param nhưng trả `nil`:
  ```go
  func(ctx context.Context, token string, headers http.Header) (string, bool, map[string]string, error) {
      return userID, false, nil, nil
  }
  ```
- Metadata được lưu trong `WebsocketAuth` struct và có thể truy cập trong `HandleMessage` callback qua `auth.Metadata`.

---

## 7. Message Acknowledgment (ACK) — Server-side

### Mô tả
Cho phép server gửi message và **chờ client xác nhận đã nhận** (acknowledgment). Quan trọng cho chat apps, financial notifications, hoặc bất kỳ message nào cần đảm bảo delivery.

### API mới

```go
// Gửi tới 1 session, chờ ACK trong 5 giây
acked, err := svc.SendToSessionWithAck(ctx, "user-123", "session-abc", "payment_update", payload, 5*time.Second)
if !acked {
    // Client chưa xác nhận trong 5s — có thể retry hoặc fallback
}

// Gửi tới tất cả sessions của user, chờ ACK từ BẤT KỲ session nào
acked, err := svc.SendToUserWithAck(ctx, "user-123", "payment_update", payload, 5*time.Second)
```

### Protocol

**Server → Client:**
```json
{ "t": "payment_update", "a": "ack_xxxxxxxxxxxx", "b": <payload> }
```

**Client → Server (ACK response):**
```json
{ "t": "__ack__", "ackId": "ack_xxxxxxxxxxxx" }
```

### AckManager — Cơ chế nội bộ

```go
// 1. Server tạo ackID + channel chờ
ackID, ch := ackManager.CreateAck()

// 2. Gửi message với ackID cho client
conn.Send(wsRes)  // wsRes.AckId = ackID

// 3. Chờ client gửi ACK hoặc timeout
acked := ackManager.WaitForAck(ackID, ch, timeout)

// 4. Khi client gửi message type "__ack__", handler gọi:
ackManager.ResolveAck(ackID)  // → close channel → WaitForAck returns true

// 5. Graceful shutdown — cancel tất cả pending ACKs
ackManager.Shutdown()  // → close tất cả channels → unblock goroutines đang chờ
```

### Files mới
| File | Mô tả |
|------|--------|
| `core/service/websocket/ack-manager/ack_manager.go` | `AckManager` struct — quản lý pending ACK map (thread-safe), `CreateAck()`, `ResolveAck()`, `WaitForAck()`, `Shutdown()` |
| `core/service/websocket/mediator/service/1.send_with_ack.go` | `SendToSessionWithAck()`, `SendToUserWithAck()` implementation |

### Files thay đổi
| File | Thay đổi |
|------|----------|
| `core/service/websocket/0.message_type.go` | Thêm `MessageTypeAck = "__ack__"`, thêm field `AckId string` vào `WebsocketResponse` (msgpack tag: `"a"`) |
| `core/service/websocket/0.ws_service.go` | Thêm `SendToSessionWithAck()`, `SendToUserWithAck()` vào `WsService` |
| `core/delivery/module.go` | Thêm vào `ExportedServices` |
| `core/service/websocket/client-msg-handler/0_main_handler.go` | Thêm case `MessageTypeAck` — parse `ackId` từ message body, gọi `ackManager.ResolveAck()` |
| `core/service/websocket/mediator/service/0.new.go` | Thêm dependency `ackManager *ackmanager.AckManager` |
| `app/wire_gen.go` | Cập nhật Wire DI — tạo `ackmanager.New()`, inject vào `mediatorsvc.New()` và `clientmsghandler.New()` |

### Chú ý
- **ACK chỉ hoạt động với connections trên cùng container** vì dùng `ConnectionManager.GetConnection()` trực tiếp, **không đi qua pub/sub**. Trong multi-container deployment, `SendToSessionWithAck` chỉ works nếu session đang trên container gọi API.
- `SendToUserWithAck` gửi tới **tất cả sessions** nhưng chỉ cần **1 ACK** từ bất kỳ session nào.
- Timeout: Nếu client không gửi ACK trong thời gian cho phép, method trả về `acked = false` (không phải error).
- **Frontend SDK cần implement**: Detect message có field `ackId` → xử lý xong → gửi lại message type `__ack__` với `ackId` tương ứng. Chi tiết xem `FRONTEND_SDK_TODO.md`.
- AckManager dùng `sync.RWMutex` + `map[string]chan struct{}` — thread-safe, auto cleanup khi timeout.
- **Graceful Shutdown**: Gọi `ackManager.Shutdown()` trước khi đóng connections để cancel tất cả pending ACKs. Method này close tất cả channels trong pending map → unblock mọi goroutine đang chờ trong `WaitForAck()` (trả về `false`). Nên gọi trong shutdown sequence trước khi close connections để tránh goroutine leak.

---

## 8. Sửa lỗi OnCloseStuffFn — Map key issue

### Mô tả
Fix bug khi dùng `WebsocketAuth` struct làm map key. Sau khi thêm field `Metadata map[string]string`, struct không còn comparable → không thể dùng làm map key.

### Thay đổi

**Trước:**
```go
type onCloseStuffFn struct {
    fnsMap map[voAuth.WebsocketAuth]func(auth voAuth.WebsocketAuth)
}
```

**Sau:**
```go
type onCloseStuffFn struct {
    fnsMap   map[string]func(auth voAuth.WebsocketAuth)  // key = "userID:instanceID"
    authsMap map[string]voAuth.WebsocketAuth               // lưu auth object riêng
}

func authKey(auth voAuth.WebsocketAuth) string {
    return auth.UserID + ":" + auth.InstanceID
}
```

### File thay đổi
- `core/service/websocket/ws-event-trigger/0.2.new_on_close.go`

---

## Tổng hợp Breaking Changes

| Thay đổi | Ảnh hưởng | Cách migration |
|----------|-----------|----------------|
| `InspectToken` thêm param `headers http.Header` + return `metadata map[string]string` | **Tất cả code implement `InspectToken`** | Thêm param `headers http.Header` (thứ 3) + return `nil` nếu không cần metadata |
| `MetricsHandler()` thêm vào `ModuleDelivery` interface | Code implement `ModuleDelivery` bên ngoài SDK | Thêm method `MetricsHandler()` vào implementation |
| `WebsocketAuth` thêm field `Metadata` | Không trực tiếp breaking (zero value = nil) | Không cần action |
| `ActiveConnection` thêm field `ConnectedAt` | Connections cũ có zero time | Không ảnh hưởng logic hiện tại |
| `WebsocketResponse` thêm field `AckId` | Client nhận thêm field `a` trong message | Client bỏ qua field không biết → tương thích ngược |

---

## Dependencies mới (go.mod)

```
github.com/prometheus/client_golang
go.opentelemetry.io/otel/exporters/prometheus
go.opentelemetry.io/otel/sdk/metric
```

---

## Files mới tạo (tổng cộng 18 files)

| File | Mô tả |
|------|--------|
| `core/service/websocket/ack-manager/ack_manager.go` | ACK manager (pending map + timeout) |
| `core/service/websocket/broadcast-msg-handler/1_broadcast.go` | Broadcast handler |
| `core/service/websocket/broadcast-msg-handler/1_disconnect_session.go` | DisconnectSession handler |
| `core/service/websocket/broadcast-msg-handler/1_disconnect_user.go` | DisconnectUser handler |
| `core/service/websocket/broadcast-msg-handler/1_send_to_users.go` | SendToUsers handler |
| `core/service/websocket/mediator/service/1.broadcast.go` | Broadcast service |
| `core/service/websocket/mediator/service/1.check_online_multiple.go` | CheckOnlineMultiple service |
| `core/service/websocket/mediator/service/1.disconnect_session.go` | DisconnectSession service |
| `core/service/websocket/mediator/service/1.disconnect_user.go` | DisconnectUser service |
| `core/service/websocket/mediator/service/1.get_user_sessions.go` | GetUserSessions service |
| `core/service/websocket/mediator/service/1.send_notification_to_users.go` | SendToUsers service |
| `core/service/websocket/mediator/service/1.send_with_ack.go` | SendWithAck service |
| `core/repository/impl-dynamodb/active_conn/count_active_connections_batch.go` | DynamoDB batch count |
| `core/repository/impl-dynamodb/active_conn/get_active_connections.go` | DynamoDB get connections |
| `core/repository/impl-postgres/active_conn/count_active_connections_batch.go` | PostgreSQL batch count |
| `core/repository/impl-postgres/active_conn/get_active_connections.go` | PostgreSQL get connections |
| `pkg/metrics/metrics.go` | OTEL Prometheus metrics |
| `FRONTEND_SDK_TODO.md` | Hướng dẫn Frontend SDK implement ACK |

---

## Quick Reference — Tất cả API mới

```go
svc := pw.Services().Websocket()

// === Force Disconnect ===
svc.DisconnectSession(ctx, userID, instanceID)
svc.DisconnectUser(ctx, userID)

// === Batch Send ===
svc.SendToUsers(ctx, []string{...}, msgType, payload)

// === Broadcast ===
svc.Broadcast(ctx, delivery.BroadcastAll, msgType, payload)
svc.Broadcast(ctx, delivery.BroadcastAuthOnly, msgType, payload)
svc.Broadcast(ctx, delivery.BroadcastAnonOnly, msgType, payload)

// === Presence ===
statuses, _ := svc.CheckOnlineMultiple(ctx, []string{...})
sessions, _ := svc.GetUserSessions(ctx, userID)

// === Message ACK ===
acked, _ := svc.SendToSessionWithAck(ctx, userID, instanceID, msgType, payload, timeout)
acked, _ := svc.SendToUserWithAck(ctx, userID, msgType, payload, timeout)

// === Metrics ===
pw.MetricsHandler() // http.Handler cho /metrics
```
