# Pipewave SDK — 7 New Features Design Spec

## Overview

Add 7 features to the Pipewave Go SDK across 3 phases, extending `ExportedServices` interface and supporting infrastructure.

---

## Phase 1 — Core Missing Features

### 1. DisconnectSession / DisconnectUser

**Interface additions** on `ExportedServices`:
```go
DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError
DisconnectUser(ctx context.Context, userID string) aerror.AError
```

**Implementation:**
- Add new pub/sub channels: `channelDisconnectSession`, `channelDisconnectUser`
- Add params types: `DisconnectSessionParams`, `DisconnectUserParams`
- `MsgCreator` gets `DisconnectSession()`, `DisconnectUser()` methods
- `PubsubHandler` gets corresponding handlers that call `ConnectionManager.GetConnection()` / `GetAllUserConn()` then `conn.Close()`
- `mediatorSvc` publishes disconnect via broadcast (same pattern as SendToUser)
- Trigger `OnCloseStuffFn` handlers normally (handled by existing close flow)

### 2. SendToUsers (Batch Send)

**Interface addition** on `ExportedServices`:
```go
SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError
```

**Implementation:**
- Add new pub/sub channel: `channelSendToUsers`
- Add params type: `SendToUsersParams { UserIds []string, MsgType string, Payload []byte }`
- `MsgCreator` gets `SendToUsers()` method
- `PubsubHandler` handler iterates `UserIds`, for each calls `ConnectionManager.GetAllUserConn()` and sends
- Single pub/sub publish instead of N separate publishes

### 3. Broadcast (Global with Target Filter)

**New types:**
```go
type BroadcastTarget int
const (
    BroadcastAll       BroadcastTarget = iota
    BroadcastAuthOnly
    BroadcastAnonOnly
)
```

**Interface addition** on `ExportedServices`:
```go
Broadcast(ctx context.Context, target BroadcastTarget, msgType string, payload []byte) aerror.AError
```

**Implementation:**
- Add new pub/sub channel: `channelBroadcast`
- Add params type: `BroadcastParams { Target int, MsgType string, Payload []byte }`
- `PubsubHandler` handler switches on target:
  - `BroadcastAll`: iterate all connections via `GetAllConnections()`
  - `BroadcastAuthOnly`: iterate user connections only (keys of `userConn` map)
  - `BroadcastAnonOnly`: iterate anonymous connections via `GetAllAnonymousConn()`

---

## Phase 2 — Enhanced Observability & Presence

### 4. Presence System

**Interface additions** on `ExportedServices`:
```go
CheckOnlineMultiple(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError)
GetUserSessions(ctx context.Context, userID string) ([]SessionInfo, aerror.AError)
```

**New type:**
```go
type SessionInfo struct {
    InstanceID  string
    ConnectedAt time.Time
    IsAnonymous bool
}
```

**Implementation:**
- `CheckOnlineMultiple`: batch query `ActiveConnStore.CountActiveConnections()` for each userID. Add `CountActiveConnectionsBatch(ctx, userIDs)` to `ActiveConnStore` interface for efficiency.
- `GetUserSessions`: add `GetActiveConnections(ctx, userID)` to `ActiveConnStore` interface, returns `[]ActiveConnection` entities. Map to `[]SessionInfo`.
- Add `ConnectedAt` field to `ActiveConnection` entity (set on `AddConnection`).

### 5. OTEL Metrics Export

**Interface addition** on `ModuleDelivery`:
```go
MetricsHandler() http.Handler
```

**Metrics using OTEL Metrics SDK:**
- `pipewave_active_connections` (gauge, labels: type=user|anonymous)
- `pipewave_messages_sent_total` (counter, labels: target=session|user|users|broadcast|anonymous)
- `pipewave_messages_received_total` (counter)
- `pipewave_connection_duration_seconds` (histogram)
- `pipewave_worker_pool_utilization` (gauge)
- `pipewave_pubsub_messages_total` (counter)

**Implementation:**
- Create metrics provider in `pkg/metrics/` or extend existing `provider/otel-provider/`
- Register meters in service layer where events occur
- `MetricsHandler()` returns OTEL Prometheus exporter HTTP handler (bridge)

### 6. Connection Metadata

**Changes to `WebsocketAuth`:**
```go
type WebsocketAuth struct {
    UserID     string
    InstanceID string
    Metadata   map[string]string  // NEW
}
```

**Changes to `InspectToken` signature in `Fns`:**
```go
InspectToken func(ctx context.Context, token string) (username string, IsAnonymous bool, metadata map[string]string, err error)
```

**Implementation:**
- Update `WebsocketAuth` struct (add Metadata field)
- Update `InspectToken` function signature in `Fns`
- Pass metadata through connection establishment flow
- Metadata accessible via `auth.Metadata` in all callbacks
- In-memory only, no persistence needed

---

## Phase 3 — Advanced Features

### 7. Message Acknowledgment

**Interface addition** on `ExportedServices`:
```go
SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)
SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)
```

**Implementation:**
- ACK manager: in-memory map of `ackID -> chan struct{}`
- When sending with ACK, generate unique `ackID`, add to pending map, include in message
- Message format adds `ackId` field to `WebsocketResponse`
- Client sends back `__ack__` message type with the `ackId`
- `HandleMessage` in client-msg-handler detects `__ack__` type, resolves pending channel
- If timeout expires, return `acked=false`
- Cross-container: ACK only works when caller is on same container as connection (local operation, no pub/sub needed for ACK response)

**Frontend SDK requirement:** Client must detect `ackId` in messages and send back `{ "t": "__ack__", "ackId": "<id>" }`. Documented in `FRONTEND_SDK_TODO.md`.

---

## Decisions Made

| Decision | Choice | Reason |
|----------|--------|--------|
| Room/Channel | Out of scope | Business logic, client implements |
| GetOnlineUsers | Removed | Scale concern, caller knows userIDs |
| Typed Message Handlers | Removed | Keep simple, one HandleMessage |
| Connection Metadata storage | In-memory only | Set once via InspectToken |
| Set/GetConnectionMetadata API | Removed | Metadata via WebsocketAuth in callbacks |
| Metrics approach | OTEL Metrics SDK | Consistent with existing OTEL tracing |
| ACK mechanism | Server-side, client sends __ack__ | Frontend SDK implements ACK sending |
