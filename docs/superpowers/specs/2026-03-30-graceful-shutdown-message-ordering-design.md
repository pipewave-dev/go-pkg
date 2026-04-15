# Graceful Container Shutdown & Message Ordering

**Date:** 2026-03-30
**Status:** Approved

---

## Overview

Khi một container gracefully shutdown, các WebSocket connection cần được chuyển sang trạng thái `WsStatusTransferring` để client có thể reconnect đến container khác mà không mất message. Đồng thời cần xử lý race condition khi client reconnect trong lúc pending messages đang được drain.

---

## 1. Repository Layer

### New Method: `UpdateStatusTransferring`

Thêm vào `ActiveConnStore` interface (`core/repository/active_conn.go`):

```go
// UpdateStatusTransferring atomically sets Status=WsStatusTransferring and clears HolderID="".
// Used exclusively during graceful container shutdown.
UpdateStatusTransferring(ctx context.Context, userID string, instanceID string) aerror.AError
```

Implement cho cả `impl-dynamodb/active_conn/` và `impl-postgres/active_conn/`.

**Semantic:** Khác với `UpdateStatus` (chỉ update Status field), method này update đồng thời cả `Status=WsStatusTransferring` và `HolderID=""` trong một operation duy nhất. HolderID rỗng báo hiệu "không container nào đang hold session này — client được phép reconnect vào bất kỳ container nào".

---

## 2. Shutdown Flow

### File: `mediator/service/3.shutdown.go`

Thứ tự thực hiện thay đổi:

```
Before (current):
  1. MarkShuttingDown()
  2. ackManager.Shutdown()
  3. Close all connections → onClose → RemoveConnection

After (new):
  1. MarkShuttingDown()
  2. ackManager.Shutdown()
  3. For each authenticated connection in connectionMgr:
     a. activeConnRepo.UpdateStatusTransferring(ctx, userID, instanceID)
     b. msgHubSvc.Register(userID, instanceID, onExpired)
  4. Close all connections (last)
```

**Lý do close sau cùng:** Nếu DB update chưa hoàn thành (network slow), WebSocket vẫn còn sống để receive và buffer message. Chỉ khi DB đã ghi nhận Transferring thì mới đóng connection để client biết cần reconnect.

**onExpired callback trong shutdown:** Khi TTL hết mà client chưa reconnect, gọi `activeConnRepo.RemoveConnection()` để cleanup record.

---

## 3. `onClose` Behavior During Shutdown

### File: `mediator/delivery/0.new.go` — `onCloseRegister()`

```go
// Before:
if auth.IsAnonymous() || d.shutdownSignal.IsShuttingDown() {
    activeConnRepo.RemoveConnection(...)
    return
}

// After:
if auth.IsAnonymous() {
    activeConnRepo.RemoveConnection(...)
    return
}
if d.shutdownSignal.IsShuttingDown() {
    return  // Skip: shutdown pre-processing đã handle DB + msgHub registration
}
// Normal temp-disconnect path (unchanged)...
```

**Lý do:** Khi `IsShuttingDown()=true`, shutdown flow (step 3 ở trên) đã update DB sang Transferring và register msgHub TRƯỚC khi close connections. Nếu `onClose` tiếp tục gọi `RemoveConnection`, nó sẽ xóa mất record Transferring mà client cần để reconnect.

---

## 4. Routing khi `WsStatusTransferring`

### File: `mediator/service/99.helper.go` — `findSessionConn`

**Vấn đề:** `findSessionConn.findThenAction()` hiện tại gọi `targetContainerAction([]string{actConn.HolderID})`. Khi `HolderID=""` (Transferring state), pubsub publish đến empty string sẽ fail/drop message.

**Fix:** Thêm `transferringAction` callback vào `findSessionConn`, xử lý empty HolderID:

```go
type findSessionConn struct {
    // ...existing fields...
    transferringAction func() aerror.AError  // new: called when HolderID="" (Transferring state)
}

func (f *findSessionConn) findThenAction() aerror.AError {
    // ...check local connection first (unchanged)...

    actConn, aErr := f.activeConnRepo.GetInstanceConnection(...)
    if aErr != nil { ... }

    // New: handle Transferring state (empty HolderID)
    if actConn.HolderID == "" {
        if f.transferringAction != nil {
            return f.transferringAction()
        }
        f.callbackNotfound()
        return nil
    }

    f.targetContainerAction([]string{actConn.HolderID})
    return nil
}
```

Callers trong `mediatorSvc` cung cấp `transferringAction` là closure tự wrap message và save vào msgHub:

```go
// Trong mediatorSvc.SendToSession():
transferringAction: func() aerror.AError {
    id := fn.NewUUID()
    wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(), "", wsSv.MessageType(pl.MsgType), pl.Payload)
    return m.msgHubSvc.Save(ctx, userID, instanceID, wsRes)
},
```

**Lý do dùng callback thay vì pre-built bytes:** Tầng `mediatorSvc` chỉ có `msgType` + `payload` raw; việc wrap thành WS frame (`WrapperBytesToWebsocketResponse`) xảy ra ở tầng `broadcastMsgHandler`. Callback cho phép mỗi caller tự quyết định cách wrap phù hợp.

---

## 5. `onNewRegister` — Handle `WsStatusTransferring`

### File: `mediator/delivery/0.new.go` — `onNewRegister()`

```go
// Before: chỉ check WsStatusTempDisconnected
if actConn.Status == voWs.WsStatusTempDisconnected {
    d.wsService.ResumeSession(ctx, actConn.HolderID, ...)
}

// After:
switch actConn.Status {
case voWs.WsStatusTempDisconnected:
    // Signal old container để deregister msgHub timer
    d.wsService.ResumeSession(ctx, actConn.HolderID, ...)
case voWs.WsStatusTransferring:
    // Old container đang shutdown, HolderID rỗng → không cần signal
    // AddConnection + Consume phía dưới sẽ handle
}
```

---

## 6. Send Ordering — `DrainableConn` Interface

### Problem

Khi client reconnect, flow hiện tại:
```
connectionMgr.AddConnection(conn)  ← connection visible, concurrent sends bypass vào conn.Send()
msgHubSvc.Consume() + send pending ← quá muộn, message ordering bị sai
```

### Solution: `drainMu sync.RWMutex`

**New interface** (`core/service/websocket/2.connection_type.go`):

```go
// DrainableConn is implemented by connections that support drain-before-send ordering.
type DrainableConn interface {
    WebsocketConn
    BeginDrain()                    // acquires drainMu.Lock() — blocks all Send() calls
    EndDrain()                      // releases drainMu.Unlock()
    SendDirect(payload []byte) error // writes to socket directly, bypasses drainMu (use only during drain)
}
```

**`GobwasConnection`** (`server/gobwas/1_type.go`):

```go
type GobwasConnection struct {
    // ...existing fields...
    drainMu sync.RWMutex
}

func (c *GobwasConnection) Send(payload []byte) error {
    c.drainMu.RLock()         // blocks if drain is in progress
    defer c.drainMu.RUnlock()
    return c.server.send(c, payload)
}

func (c *GobwasConnection) BeginDrain() { c.drainMu.Lock() }
func (c *GobwasConnection) EndDrain()   { c.drainMu.Unlock() }
func (c *GobwasConnection) SendDirect(payload []byte) error {
    return c.server.send(c, payload)  // no drainMu, caller holds WLock
}
```

**`onNewRegister`** — thứ tự mới:

```go
// 1. BeginDrain TRƯỚC khi add to connectionMgr
if dc, ok := connection.(DrainableConn); ok {
    dc.BeginDrain()
    defer dc.EndDrain()  // tự động release sau khi hàm return
}

// 2. Add to connectionMgr (concurrent sends sẽ block ở drainMu.RLock)
d.connectionMgr.AddConnection(connection)
d.rateLimiter.New(auth)

// 3. Consume + send pending (WLock held → concurrent sends blocked)
msgs, _ := d.msgHubSvc.Consume(ctx, auth.UserID, auth.InstanceID)
for _, msg := range msgs {
    if dc, ok := connection.(DrainableConn); ok {
        dc.SendDirect(msg)  // bypass Send() để tránh deadlock
    } else {
        connection.Send(msg)  // fallback (e.g. long polling nếu không implement DrainableConn)
    }
}

// 4. defer EndDrain() fires → WLock release → blocked Send() goroutines tiếp tục
//    Tất cả message mới đến SAU pending messages ✓
```

---

## 7. Long Polling Connection

`LongPollingConn` cũng cần implement `DrainableConn` tương tự `GobwasConnection` để đảm bảo ordering consistency. Nếu long polling không dùng concurrent sends (do nature của HTTP request/response), có thể implement `BeginDrain/EndDrain` là no-op nhưng nên implement đúng để consistency.

---

## Summary: Files to Change

| File | Change |
|------|--------|
| `core/repository/active_conn.go` | Add `UpdateStatusTransferring` to interface |
| `impl-dynamodb/active_conn/update_status.go` (hoặc file mới) | Implement `UpdateStatusTransferring` |
| `impl-postgres/active_conn/update_status.go` (hoặc file mới) | Implement `UpdateStatusTransferring` |
| `mediator/service/3.shutdown.go` | Rewrite shutdown flow |
| `mediator/delivery/0.new.go` | Fix `onCloseRegister` + `onNewRegister` |
| `mediator/service/99.helper.go` | Add `msgHubSvc` + `wrappedMsg` to `findSessionConn`, handle empty HolderID |
| `mediator/service/1.send_notification_to_session.go` | Pass `transferringAction` closure to `findSessionConn` |
| `core/service/websocket/2.connection_type.go` | Add `DrainableConn` interface |
| `server/gobwas/1_type.go` | Add `drainMu`, implement `DrainableConn` |
| `server/gobwas/1_server.go` | Ensure `send()` usable from `SendDirect` |
| `mediator/delivery/3.long_polling.go` | Implement `DrainableConn` on LongPolling conn |

---

## Key Invariants

1. **DB update trước, close sau**: Đảm bảo client reconnect luôn thấy trạng thái Transferring trong DB.
2. **onClose skip khi IsShuttingDown**: Tránh xóa record Transferring.
3. **HolderID rỗng → msgHub save trực tiếp**: Không route qua pubsub khi không có holder.
4. **BeginDrain trước AddConnection**: Đảm bảo không có concurrent send nào qua trước pending.
5. **SendDirect trong drain**: Tránh deadlock với drainMu.
