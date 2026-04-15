# MessageHub Design Spec

**Date:** 2026-03-27
**Status:** Approved

---

## Problem

When a WebSocket session temporarily disconnects (e.g., 3s network blip), any messages sent to that session during the gap are silently dropped. When the session reconnects, those messages are lost.

---

## Goals

- Buffer messages for temporarily disconnected sessions
- Deliver buffered messages on reconnect
- Support cross-container routing (messages may arrive at a different container than the one holding the session)
- Drop messages automatically if the session does not reconnect within a configurable TTL
- No changes to the behavior for permanently disconnected sessions

---

## Non-Goals

- Message deduplication on the client side
- Ordered delivery guarantees beyond DynamoDB sort key ordering
- Buffering for anonymous sessions
- Message count or payload size caps per session buffer

---

## Architecture Overview

```
[On Close - P1]
  connectionMgr.Remove(auth)
  activeConnRepo.UpdateStatus(TempDisconnected)   ← keeps record + HolderID intact
  msgHubSvc.Register(userID, instanceID, onExpired) ← starts in-memory ExpiredTimer

[On New - P2]
  actConn = activeConnRepo.GetInstanceConnection(...)
  if actConn.Status == TempDisconnected:
      if actConn.HolderID == self → msgHubSvc.Deregister(...)   (cancel timer locally)
      else → broadcast ResumeSession to P1                        (P1 cancels timer)
  activeConnRepo.AddConnection(...)   ← upsert: new HolderID + status reset to Connected
  connectionMgr.Add(conn)
  msgs = msgHubSvc.Consume(...)
  conn.Send(msgs...)

[SendToSession - P1 receives pubsub]
  conn, ok = connectionMgr.GetConnection(auth)
  if !ok:
      if msgHubSvc.IsRegistered(userID, instanceID):
          msgHubSvc.Save(ctx, userID, instanceID, payload)
      else:
          slog.Warn("session not found, dropping message")
      return

[SendToUser - P1 receives pubsub]
  conns = connectionMgr.GetAllUserConn(userID)
  for conn in conns: conn.Send(msg)
  for instanceID in msgHubSvc.GetSessions(userID):
      slog.Warn("session temp disconnected, buffering message", ...)
      msgHubSvc.Save(ctx, userID, instanceID, payload)

[ResumeSession handler - P1 receives pubsub]
  msgHubSvc.Deregister(userID, instanceID)   ← cancels in-memory timer
```

### Terminology

`instanceID` throughout this spec is the same field as `SessionID` in the `ActiveConnection` DynamoDB entity. The codebase uses `InstanceID` in `voAuth.WebsocketAuth`; the DB entity uses `SessionID` as its sort key. They refer to the same value.

### Container crash resilience

If the container holding the `ExpiredTimer` (P1) crashes, the timer is lost. The `ActiveConnection` record is cleaned up by DynamoDB TTL (set at the time of temp disconnect). Pending messages in the `PendingMessage` table are cleaned up by their own DynamoDB TTL (same duration as session TTL). This is an accepted known limitation.

In the window between P1 crash-restart and DB TTL expiry, new messages routed to P1 will hit `IsRegistered = false` and be dropped with `slog.Warn`. This is acceptable — same behavior as current silent drop, but now visible in logs.

### Handoff race window

When a session reconnects (P2), there is a narrow async window during which P1 (old holder) may still buffer a message after P2's `Consume` has already run. This is an inherent property of async pubsub and results in at-most-once delivery during the handoff transition. Worst case: one in-flight message at the exact reconnect moment is lost. This is accepted.

### Graceful shutdown

`mediatorSvc.Shutdown()` calls `conn.Close()` on all connections, which triggers `onCloseStuff` → `onCloseRegister`. Without intervention, every session would enter the temp-disconnect path, causing a flood of DynamoDB writes.

Fix: `serverDelivery` must expose a `MarkShuttingDown()` method (sets an `atomic.Bool`). `mediatorSvc.Shutdown()` calls this before closing connections. `onCloseRegister` checks this flag and uses the permanent-disconnect path (remove from DB immediately) when shutting down.

---

## New Interfaces

### `MessageHubSvc`

```go
// core/service/websocket/msg-hub/interface.go

type MessageHubSvc interface {
    // Register starts ExpiredTimer; onExpired is called when the timer fires.
    Register(userID, instanceID string, onExpired func())
    // Deregister cancels the timer and removes the session from the registry.
    Deregister(userID, instanceID string)
    // IsRegistered checks whether a session is in the temp-disconnect registry.
    IsRegistered(userID, instanceID string) bool
    // GetSessions returns all temp-disconnected instanceIDs for a user.
    GetSessions(userID string) []string

    // Save buffers a message for a temp-disconnected session in the DB.
    Save(ctx context.Context, userID, instanceID string, msg []byte) error
    // Consume drains all pending messages and deletes them from the DB.
    Consume(ctx context.Context, userID, instanceID string) ([][]byte, error)
}
```

Internal implementation uses a nested map for the registry:

```go
type msgHubSvc struct {
    mu       sync.RWMutex
    registry map[string]map[string]context.CancelFunc // userID -> instanceID -> cancelFn
    repo     repository.PendingMessageRepo
    ttl      time.Duration // from config provider
}
```

### `PendingMessageRepo`

```go
// core/repository/pending_message.go

type PendingMessageRepo interface {
    Create(ctx context.Context, userID, instanceID string, sendAt time.Time, message []byte) error
    GetAll(ctx context.Context, userID, instanceID string) ([][]byte, error)
    DeleteAll(ctx context.Context, userID, instanceID string) error
}
```

DynamoDB table structure:
- Hash key: `userID + ":" + instanceID`
- Sort key: `sendAt` (server receive time as int64 Unix nano — used for ordering)
- TTL attribute: same duration as the session TTL from config provider
- `GetAll` returns messages ordered ascending by `sendAt`

### Updates to existing interfaces

```go
// core/repository/active_conn.go — new method (additive, non-breaking)
UpdateStatus(ctx context.Context, userID, instanceID string, status voWs.WsStatus) aerror.AError

// core/repository/0.all_repo.go — new method (additive, non-breaking)
PendingMessage() PendingMessageRepo

// core/service/websocket/0.ws_service.go — new method (additive; all existing WsService
// implementations must add a stub or implement this method)
ResumeSession(ctx context.Context, targetContainerID, userID, instanceID string) aerror.AError
```

### Broadcast types

```go
// core/service/websocket/broadcast/1.1.params_type.go
type ResumeSessionParams struct {
    UserID     string
    InstanceID string
}
// + Marshal / Unmarshal via msgpack

// core/service/websocket/broadcast/1.0.msg_enum.go
msgTResumeSession msgType = "ResumeSession"
```

---

## Files to Create

| File | Purpose |
|------|---------|
| `core/service/websocket/msg-hub/interface.go` | `MessageHubSvc` interface |
| `core/service/websocket/msg-hub/service.go` | Implementation (registry + timer + DB delegation) |
| `core/service/websocket/msg-hub/wire.go` | Constructor |
| `core/repository/pending_message.go` | `PendingMessageRepo` interface |
| `core/service/websocket/broadcast-msg-handler/1_resume_session.go` | `ResumeSession` pubsub handler |
| `core/service/websocket/mediator/service/1.resume_session.go` | `ResumeSession` mediator method |

## Files to Modify

| File | Change |
|------|--------|
| `core/repository/0.all_repo.go` | Add `PendingMessage() PendingMessageRepo` |
| `core/repository/active_conn.go` | Add `UpdateStatus()` |
| `core/service/websocket/0.ws_service.go` | Add `ResumeSession()` |
| `core/service/websocket/broadcast/1.0.msg_enum.go` | Add `msgTResumeSession` |
| `core/service/websocket/broadcast/1.1.params_type.go` | Add `ResumeSessionParams` |
| `core/service/websocket/broadcast/create_msg.go` | Add `ResumeSession` factory method |
| `core/service/websocket/broadcast-msg-handler/0_main_handler.go` | Inject `msgHubSvc` |
| `core/service/websocket/broadcast-msg-handler/1_send_to_session.go` | Check registry; slog.Warn on drop |
| `core/service/websocket/broadcast-msg-handler/1_send_to_user.go` | Buffer temp sessions; slog.Warn |
| `core/service/websocket/broadcast-msg-handler/wire.go` | Inject `msgHubSvc` |
| `core/service/websocket/mediator/service/0.new.go` | Inject `msgHubSvc`; add `MarkShuttingDown()` call in `Shutdown()` |
| `core/service/websocket/mediator/service/wire.go` | Inject `msgHubSvc` |
| `core/service/websocket/mediator/service/3.shutdown.go` | Call `delivery.MarkShuttingDown()` before closing connections |
| `core/service/websocket/mediator/delivery/0.new.go` | Add `shuttingDown atomic.Bool`; update `onCloseRegister` + `onNewRegister` |
| `core/service/websocket/mediator/delivery/wire.go` | Inject `msgHubSvc`; expose `MarkShuttingDown()` |

---

## Key Flow Details

### `onCloseRegister` (delivery/0.new.go)

```
permanent path (anonymous session OR d.shuttingDown == true):
  1. connectionMgr.RemoveConnection(auth)
  2. rateLimiter.Remove(auth)
  3. activeConnRepo.RemoveConnection(...)   ← existing behavior

temp-disconnect path (authenticated session, not shutting down):
  1. connectionMgr.RemoveConnection(auth)
  2. rateLimiter.Remove(auth)
  3. activeConnRepo.UpdateStatus(ctx, userID, instanceID, WsStatusTempDisconnected)
  4. msgHubSvc.Register(userID, instanceID, func() {
         // onExpired: timer fired, session never reconnected
         activeConnRepo.RemoveConnection(ctx, userID, instanceID)
         // pending messages in DB cleaned up by their own TTL
     })
```

### `onNewRegister` (delivery/0.new.go)

```
1. Check for duplicate in-memory connection → close old if found (existing)
2. actConn = activeConnRepo.GetInstanceConnection(ctx, userID, instanceID)
   if actConn != nil && actConn.Status == WsStatusTempDisconnected:
       if actConn.HolderID == currentContainerID:
           msgHubSvc.Deregister(userID, instanceID)       // local cancel
       else:
           wsService.ResumeSession(ctx, actConn.HolderID, userID, instanceID)  // pubsub
3. activeConnRepo.AddConnection(...)   // upsert: new HolderID, status reset to Connected
4. connectionMgr.AddConnection(conn)
5. rateLimiter.New(auth)
6. msgs, _ = msgHubSvc.Consume(ctx, userID, instanceID)
   for _, msg := range msgs: conn.Send(msg)
```

### `broadcastMsgHandler.SendToSession`

```
conn, ok = connectionMgr.GetConnection(auth)
if !ok:
    if msgHubSvc.IsRegistered(userID, instanceID):
        msgHubSvc.Save(ctx, userID, instanceID, payload.Payload)
    else:
        slog.Warn("SendToSession: session not found, dropping message",
            slog.String("userID", userID),
            slog.String("instanceID", instanceID))
    return
conn.Send(wsRes)
```

### `broadcastMsgHandler.SendToUser`

```
conns = connectionMgr.GetAllUserConn(userID)
wsRes = WrapperBytesToWebsocketResponse(...)
for conn in conns: conn.Send(wsRes)

tempSessions = msgHubSvc.GetSessions(userID)
for _, instanceID := range tempSessions:
    slog.Warn("SendToUser: session temp disconnected, buffering",
        slog.String("userID", userID),
        slog.String("instanceID", instanceID))
    msgHubSvc.Save(ctx, userID, instanceID, payload.Payload)

if len(conns) == 0 && len(tempSessions) == 0:
    slog.Warn("SendToUser: no sessions found for user", slog.String("userID", userID))
```

---

## Error Handling

- `UpdateStatus` failure on close → log error; connection already removed from memory; DB TTL will eventually clean up the record
- `msgHubSvc.Save` failure → log error; message is lost (same as current silent drop behavior)
- `msgHubSvc.Consume` failure on reconnect → log error; session continues normally without buffered messages
- `ResumeSession` pubsub failure → log error; ExpiredTimer on P1 fires eventually and cleans up correctly (no correctness issue, only a delay)

### `Consume` contract

`Consume` executes in order: `GetAll` → `DeleteAll` → return messages.

- If crash after `GetAll` but before `DeleteAll`: next reconnect re-delivers the same messages (duplicate delivery — preferred over loss)
- If crash after `DeleteAll` but before caller sends: messages are lost
- `DeleteAll` failure → log error; messages remain in DB until TTL expiry; next reconnect may re-deliver (acceptable)

---

## Decisions

| Decision | Rationale |
|----------|-----------|
| In-memory registry on holder container | Avoids DB query on every send hot path |
| DB TTL for crash recovery | Simple, no background job needed |
| Separate `PendingMessage` table | Avoids write contention on the `ActiveConnection` record |
| `Consume`: GetAll → DeleteAll → return | Prefer duplicate delivery over message loss on crash |
| Anonymous sessions not buffered | Anonymous sessions have no stable identity to reconnect with |
| `shuttingDown` flag in `serverDelivery` | Prevents temp-disconnect DB flood during graceful shutdown |
| At-most-once delivery during handoff transition | Inherent to async pubsub; narrow window; accepted trade-off |
