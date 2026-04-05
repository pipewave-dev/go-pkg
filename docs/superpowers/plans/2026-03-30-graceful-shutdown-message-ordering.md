# Graceful Container Shutdown & Message Ordering — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement graceful container shutdown that transfers WebSocket sessions to `WsStatusTransferring` (clearing HolderID) and buffers messages via MessageHub, while guaranteeing that pending messages are drained before new messages on reconnect.

**Architecture:** Three coordinated changes: (1) a new `UpdateStatusTransferring` repo method that atomically sets Status+clears HolderID, (2) a `DrainableConn` interface with `drainMu sync.RWMutex` that blocks concurrent `Send()` calls during the pending-message drain window, (3) updated shutdown/reconnect logic in `mediatorSvc` and `serverDelivery` to use these primitives in the correct order.

**Tech Stack:** Go 1.25, DynamoDB (`aws-sdk-go-v2/feature/dynamodb`), PostgreSQL (`pgx/v5`), gobwas WebSocket, standard `sync` package.

---

## File Map

| File                                                                        | Action | Responsibility                                                       |
| --------------------------------------------------------------------------- | ------ | -------------------------------------------------------------------- |
| `core/repository/active_conn.go`                                            | Modify | Add `UpdateStatusTransferring` to interface                          |
| `core/repository/impl-dynamodb/active_conn/exprbuilder/0.3.updater.go`      | Modify | Add `UpdateStatusTransferring` DynamoDB expression builder           |
| `core/repository/impl-dynamodb/active_conn/update_status_transferring.go`   | Create | DynamoDB repo method                                                 |
| `core/repository/impl-postgres/active_conn/update_status_transferring.go`   | Create | Postgres repo method                                                 |
| `core/service/websocket/2.connection_type.go`                               | Modify | Add `DrainableConn` interface                                        |
| `core/service/websocket/server/gobwas/1_type.go`                            | Modify | Add `drainMu`, implement `DrainableConn` on `GobwasConnection`       |
| `core/service/websocket/server/gobwas/drain_test.go`                        | Create | Unit test: drain ordering                                            |
| `core/service/websocket/mediator/delivery/3.long_polling.go`                | Modify | Implement `DrainableConn` on `LongPollingConn`                       |
| `core/service/websocket/mediator/service/99.helper.go`                      | Modify | Add `transferringAction` to `findSessionConn`, handle empty HolderID |
| `core/service/websocket/mediator/service/1.send_notification_to_session.go` | Modify | Pass `transferringAction` closure                                    |
| `core/service/websocket/mediator/service/1.send_with_ack.go`                | Modify | Pass `transferringAction` closure                                    |
| `core/service/websocket/mediator/delivery/0.new.go`                         | Modify | Fix `onCloseRegister` + `onNewRegister`                              |
| `core/service/websocket/mediator/service/3.shutdown.go`                     | Modify | Rewrite shutdown flow                                                |

---

## Task 1: Add `UpdateStatusTransferring` to Repository Interface

**Files:**

- Modify: `core/repository/active_conn.go`

- [ ] **Step 1: Add method to interface**

Open `core/repository/active_conn.go` and add after the `UpdateStatus` method declaration:

```go
// UpdateStatusTransferring atomically sets Status=WsStatusTransferring and clears HolderID="".
// Used exclusively during graceful container shutdown so that any container can pick up the session on reconnect.
UpdateStatusTransferring(ctx context.Context, userID string, instanceID string) aerror.AError
```

The full interface block should look like:

```go
type ActiveConnStore interface {
    CountActiveConnections(ctx context.Context, userID string) (int, aerror.AError)
    CountTotalActiveConnections(ctx context.Context) (int64, aerror.AError)
    AddConnection(ctx context.Context, userID string, instanceID string, connectionType voWs.WsCoreType) aerror.AError
    RemoveConnection(ctx context.Context, userID string, instanceID string) aerror.AError
    UpdateHeartBeat(ctx context.Context, userID string, instanceID string) aerror.AError
    UpdateStatus(ctx context.Context, userID string, instanceID string, status voWs.WsStatus) aerror.AError
    // UpdateStatusTransferring atomically sets Status=WsStatusTransferring and clears HolderID="".
    // Used exclusively during graceful container shutdown so that any container can pick up the session on reconnect.
    UpdateStatusTransferring(ctx context.Context, userID string, instanceID string) aerror.AError
    CountActiveConnectionsBatch(ctx context.Context, userIDs []string) (map[string]int, aerror.AError)
    GetActiveConnections(ctx context.Context, userID string) ([]entities.ActiveConnection, aerror.AError)
    GetActiveConnectionsByUserIDs(ctx context.Context, userIDs []string) ([]entities.ActiveConnection, aerror.AError)
    GetInstanceConnection(ctx context.Context, userID string, instanceID string) (*entities.ActiveConnection, aerror.AError)
}
```

- [ ] **Step 2: Verify it compiles (both impls will fail — expected)**

```bash
cd /Users/ngocntr/Documents/git.ponos-tech.com/pipewave/pipewave-gopkg
go build ./core/repository/...
```

Expected: compile errors about `activeConnRepo` not implementing `ActiveConnStore` (missing `UpdateStatusTransferring`). This is correct — impls come in Tasks 2 and 3.

- [ ] **Step 3: Commit**

```bash
git add core/repository/active_conn.go
git commit -m "feat(repo): add UpdateStatusTransferring to ActiveConnStore interface"
```

---

## Task 2: DynamoDB — Implement `UpdateStatusTransferring`

**Files:**

- Modify: `core/repository/impl-dynamodb/active_conn/exprbuilder/0.3.updater.go`
- Create: `core/repository/impl-dynamodb/active_conn/update_status_transferring.go`

- [ ] **Step 1: Add expression builder method**

In `core/repository/impl-dynamodb/active_conn/exprbuilder/0.3.updater.go`, add after the `UpdateStatus` method:

```go
func (updater *ActiveConnectionUpdater) UpdateStatusTransferring(ctx context.Context, ddbClient *dynamodb.Client, userID, sessionID string) aerror.AError {
	type keySchema struct {
		UserID    string
		SessionID string
	}

	key, err := attributevalue.MarshalMap(keySchema{UserID: userID, SessionID: sessionID})
	if err != nil {
		panic(fmt.Sprintf("*ActiveConnectionUpdater.UpdateStatusTransferring marshal key error: %v", err))
	}

	update := expression.
		Set(expression.Name(FieldStatus), expression.Value(voWs.WsStatusTransferring)).
		Set(expression.Name(FieldHolderID), expression.Value(""))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		panic(fmt.Sprintf("*ActiveConnectionUpdater.UpdateStatusTransferring build expression error: %v", err))
	}

	//nolint:exhaustruct
	input := &dynamodb.UpdateItemInput{
		TableName:                 lo.ToPtr(updater.ConfigStore.Env().DynamoDB.Tables.ActiveConnection),
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err2 := ddbClient.UpdateItem(ctx, input)
	if err2 != nil {
		return aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err2)
	}
	return nil
}
```

- [ ] **Step 2: Create repo method file**

Create `core/repository/impl-dynamodb/active_conn/update_status_transferring.go`:

```go
package activeConnRepo

import (
	"context"

	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpdateStatusTransferring = "activeConnRepo.UpdateStatusTransferring"

func (r *activeConnRepo) UpdateStatusTransferring(ctx context.Context, userID string, instanceID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpdateStatusTransferring)
	defer op.Finish(aErr)

	updater := activeConnExp.ActiveConnectionUpdater{ConfigStore: r.c}
	aErr = updater.UpdateStatusTransferring(ctx, r.ddbC, userID, instanceID)
	return aErr
}
```

- [ ] **Step 3: Verify DynamoDB impl compiles**

```bash
go build ./core/repository/impl-dynamodb/...
```

Expected: no errors for DynamoDB package (Postgres still fails).

- [ ] **Step 4: Commit**

```bash
git add core/repository/impl-dynamodb/active_conn/exprbuilder/0.3.updater.go \
        core/repository/impl-dynamodb/active_conn/update_status_transferring.go
git commit -m "feat(repo/ddb): implement UpdateStatusTransferring"
```

---

## Task 3: Postgres — Implement `UpdateStatusTransferring`

**Files:**

- Create: `core/repository/impl-postgres/active_conn/update_status_transferring.go`

- [ ] **Step 1: Create repo method file**

Create `core/repository/impl-postgres/active_conn/update_status_transferring.go`:

```go
package activeConnRepo

import (
	"context"

	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpdateStatusTransferring = "activeConnRepo.UpdateStatusTransferring"

func (r *activeConnRepo) UpdateStatusTransferring(ctx context.Context, userID string, instanceID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpdateStatusTransferring)
	defer op.Finish(aErr)

	query := `
		UPDATE active_connections
		SET status = $1, holder_id = ''
		WHERE user_id = $2 AND instance_id = $3
	`

	_, err := r.pool.Exec(ctx, query, voWs.WsStatusTransferring, userID, instanceID)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
```

- [ ] **Step 2: Verify full build**

```bash
go build ./...
```

Expected: no errors — both impls now satisfy the interface.

- [ ] **Step 3: Commit**

```bash
git add core/repository/impl-postgres/active_conn/update_status_transferring.go
git commit -m "feat(repo/postgres): implement UpdateStatusTransferring"
```

---

## Task 4: `DrainableConn` Interface + `GobwasConnection` Implementation

**Files:**

- Modify: `core/service/websocket/2.connection_type.go`
- Modify: `core/service/websocket/server/gobwas/1_type.go`
- Create: `core/service/websocket/server/gobwas/drain_test.go`

- [ ] **Step 1: Write failing test**

Create `core/service/websocket/server/gobwas/drain_test.go`:

```go
package gobwas

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockDrainableConn simulates a connection that records the order in which payloads are sent.
type mockDrainableConn struct {
	drainMu sync.RWMutex
	sent    []string
	mu      sync.Mutex
}

func (c *mockDrainableConn) send(msg string) {
	c.drainMu.RLock()
	defer c.drainMu.RUnlock()
	c.mu.Lock()
	c.sent = append(c.sent, msg)
	c.mu.Unlock()
}

func (c *mockDrainableConn) sendDirect(msg string) {
	// No drainMu — caller holds WLock
	c.mu.Lock()
	c.sent = append(c.sent, msg)
	c.mu.Unlock()
}

func (c *mockDrainableConn) beginDrain() { c.drainMu.Lock() }
func (c *mockDrainableConn) endDrain()   { c.drainMu.Unlock() }

// TestDrainOrdering verifies that pending messages sent via sendDirect
// during a drain phase always appear before messages sent via send()
// from concurrent goroutines.
func TestDrainOrdering(t *testing.T) {
	const iterations = 100

	for i := 0; i < iterations; i++ {
		conn := &mockDrainableConn{}
		conn.beginDrain()

		var wg sync.WaitGroup
		var started atomic.Bool

		// Simulate 5 concurrent senders that fire as soon as drain begins.
		for j := 0; j < 5; j++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				// Spin until drain has actually started so the race is real.
				for !started.Load() {
					time.Sleep(time.Microsecond)
				}
				conn.send("new")
			}(j)
		}

		started.Store(true)

		// Give goroutines a chance to reach drainMu.RLock and block.
		time.Sleep(time.Millisecond)

		// Drain pending messages directly (WLock held).
		conn.sendDirect("pending-1")
		conn.sendDirect("pending-2")

		conn.endDrain()
		wg.Wait()

		// Verify: first two messages must be the pending ones.
		conn.mu.Lock()
		got := conn.sent
		conn.mu.Unlock()

		if len(got) < 2 {
			t.Fatalf("iter %d: expected at least 2 messages, got %d", i, len(got))
		}
		if got[0] != "pending-1" || got[1] != "pending-2" {
			t.Errorf("iter %d: expected [pending-1 pending-2 ...], got %v", i, got)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails (no drainMu yet)**

```bash
cd /Users/ngocntr/Documents/git.ponos-tech.com/pipewave/pipewave-gopkg
go test ./core/service/websocket/server/gobwas/... -run TestDrainOrdering -v
```

Expected: PASS — the mock test is self-contained and should pass immediately. This establishes the ordering contract.

- [ ] **Step 3: Add `DrainableConn` interface**

In `core/service/websocket/2.connection_type.go`, add after the `WebsocketConn` interface:

```go
// DrainableConn extends WebsocketConn with drain-phase locking.
// Connections implementing this interface allow callers to block concurrent
// Send() calls while draining pending messages in the correct order.
//
// Usage pattern:
//
//	dc.BeginDrain()            // acquire exclusive lock — all Send() calls block
//	defer dc.EndDrain()        // release lock — blocked Send() calls proceed after pending
//	for _, msg := range pending {
//	    dc.SendDirect(msg)     // write directly, bypasses drainMu to avoid deadlock
//	}
type DrainableConn interface {
	WebsocketConn
	// BeginDrain acquires an exclusive write lock. All concurrent Send() calls block until EndDrain.
	BeginDrain()
	// EndDrain releases the write lock. Blocked Send() calls resume after all SendDirect calls.
	EndDrain()
	// SendDirect writes payload to the underlying transport without acquiring drainMu.
	// MUST only be called between BeginDrain and EndDrain.
	SendDirect(payload []byte) error
}
```

- [ ] **Step 4: Add `drainMu` to `GobwasConnection` and implement `DrainableConn`**

In `core/service/websocket/server/gobwas/1_type.go`:

1. Add `sync` to imports (it's already in `1_server.go` but not `1_type.go`):

```go
import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/mailru/easygo/netpoll"
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	healthyprovider "github.com/pipewave-dev/go-pkg/provider/healthy-provider"
)
```

2. Add `drainMu` field to `GobwasConnection`:

```go
type GobwasConnection struct {
	c       configprovider.ConfigStore
	conn    net.Conn
	server  *NetpollServer
	auth    voAuth.WebsocketAuth
	desc    *netpoll.Desc
	closed  int32
	drainMu sync.RWMutex
}
```

3. Update `Send()` to acquire `RLock`:

```go
func (cl *GobwasConnection) Send(payload []byte) error {
	cl.drainMu.RLock()
	defer cl.drainMu.RUnlock()
	if cl.server != nil {
		return cl.server.send(cl, payload)
	}
	return fmt.Errorf("connection is not associated with a server")
}
```

4. Add `DrainableConn` methods:

```go
// BeginDrain acquires an exclusive lock, blocking all concurrent Send() calls.
func (cl *GobwasConnection) BeginDrain() { cl.drainMu.Lock() }

// EndDrain releases the exclusive lock, allowing blocked Send() calls to proceed.
func (cl *GobwasConnection) EndDrain() { cl.drainMu.Unlock() }

// SendDirect writes directly to the server without acquiring drainMu.
// Must only be called between BeginDrain/EndDrain.
func (cl *GobwasConnection) SendDirect(payload []byte) error {
	if cl.server != nil {
		return cl.server.send(cl, payload)
	}
	return fmt.Errorf("connection is not associated with a server")
}
```

5. Add compile-time check after existing `_ wsSv.WebsocketConn = ...` check (in `1_server.go`):

In `core/service/websocket/server/gobwas/1_server.go`, update the compile-time check block:

```go
var (
	_ wsSv.WebsocketServer = (*NetpollServer)(nil)
	_ wsSv.WebsocketConn   = (*GobwasConnection)(nil)
	_ wsSv.DrainableConn   = (*GobwasConnection)(nil)
)
```

- [ ] **Step 5: Build and run test**

```bash
go build ./core/service/websocket/...
go test ./core/service/websocket/server/gobwas/... -run TestDrainOrdering -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add core/service/websocket/2.connection_type.go \
        core/service/websocket/server/gobwas/1_type.go \
        core/service/websocket/server/gobwas/1_server.go \
        core/service/websocket/server/gobwas/drain_test.go
git commit -m "feat(ws): add DrainableConn interface and implement on GobwasConnection"
```

---

## Task 5: Implement `DrainableConn` on `LongPollingConn`

**Files:**

- Modify: `core/service/websocket/mediator/delivery/3.long_polling.go`

LongPollingConn's `Send()` publishes to a Valkey-backed queue. Drain ordering still matters: pending DB messages must be published to the queue before new messages. The `drainMu` approach works identically here.

- [ ] **Step 1: Add `drainMu` field to `LongPollingConn`**

In `core/service/websocket/mediator/delivery/3.long_polling.go`, update the struct:

```go
type LongPollingConn struct {
	auth      voAuth.WebsocketAuth
	queue     queue.Adapter
	channel   string
	done      chan struct{}
	closed    int32
	mu        sync.Mutex
	drainMu   sync.RWMutex
	idleTimer *time.Timer
	onCloseFn wsSv.OnCloseStuffFn
}
```

- [ ] **Step 2: Update `Send()` to acquire `RLock`**

Replace the existing `Send()`:

```go
// Send publishes payload to the Valkey-backed queue.
func (c *LongPollingConn) Send(payload []byte) error {
	c.drainMu.RLock()
	defer c.drainMu.RUnlock()
	if err := c.queue.Publish(context.Background(), c.channel, payload); err != nil {
		slog.Error("LP conn: failed to publish message", slog.Any("error", err), slog.Any("auth", c.auth))
		return err
	}
	return nil
}
```

- [ ] **Step 3: Add `DrainableConn` methods**

Add after `Send()`:

```go
// BeginDrain acquires an exclusive lock, blocking all concurrent Send() calls.
func (c *LongPollingConn) BeginDrain() { c.drainMu.Lock() }

// EndDrain releases the exclusive lock, allowing blocked Send() calls to proceed.
func (c *LongPollingConn) EndDrain() { c.drainMu.Unlock() }

// SendDirect publishes directly to the Valkey queue without acquiring drainMu.
// Must only be called between BeginDrain/EndDrain.
func (c *LongPollingConn) SendDirect(payload []byte) error {
	if err := c.queue.Publish(context.Background(), c.channel, payload); err != nil {
		slog.Error("LP conn: SendDirect failed", slog.Any("error", err), slog.Any("auth", c.auth))
		return err
	}
	return nil
}
```

- [ ] **Step 4: Add compile-time check**

Near the existing `var _ wsSv.WebsocketConn = (*LongPollingConn)(nil)` line, add:

```go
var _ wsSv.WebsocketConn = (*LongPollingConn)(nil)
var _ wsSv.DrainableConn  = (*LongPollingConn)(nil)
```

- [ ] **Step 5: Build**

```bash
go build ./core/service/websocket/mediator/delivery/...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add core/service/websocket/mediator/delivery/3.long_polling.go
git commit -m "feat(ws/lp): implement DrainableConn on LongPollingConn"
```

---

## Task 6: Add `transferringAction` to `findSessionConn`

**Files:**

- Modify: `core/service/websocket/mediator/service/99.helper.go`

- [ ] **Step 1: Add `transferringAction` field and handle empty HolderID**

In `core/service/websocket/mediator/service/99.helper.go`, update `findSessionConn`:

```go
type findSessionConn struct {
	ctx              context.Context
	userID           string
	instanceID       string
	callbackNotfound func()
	// transferringAction is called when the session is found in DB but HolderID is empty
	// (WsStatusTransferring). Callers should save the message to MessageHub.
	// If nil, callbackNotfound is called instead.
	transferringAction func() aerror.AError

	localAction           func()
	targetContainerAction func(containerIDs []string)

	c              configprovider.ConfigStore
	connections    wsSv.ConnectionManager
	activeConnRepo repo.ActiveConnStore
}
```

Update `findThenAction()` to handle the empty HolderID case:

```go
func (f *findSessionConn) findThenAction() aerror.AError {
	tmpAuth := voAuth.UserWebsocketAuth(f.userID, f.instanceID)
	// Check connection in memory first
	_, ok := f.connections.GetConnection(tmpAuth)
	if ok {
		f.localAction()
		return nil
	}

	// If not found, check in active connection repo
	actConn, aErr := f.activeConnRepo.GetInstanceConnection(f.ctx, f.userID, f.instanceID)
	if aErr != nil {
		if errors.Is(aErr, aerror.RecordNotFound) {
			f.callbackNotfound()
			return nil
		}
		return aErr
	}

	// Session is in WsStatusTransferring: HolderID is cleared, container is shutting down.
	// Route message to MessageHub so client receives it on reconnect.
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

- [ ] **Step 2: Build**

```bash
go build ./core/service/websocket/mediator/service/...
```

Expected: no errors (existing callers don't set `transferringAction`, which defaults to nil — `callbackNotfound` is called as before, which is safe for existing behavior).

- [ ] **Step 3: Commit**

```bash
git add core/service/websocket/mediator/service/99.helper.go
git commit -m "feat(mediator): add transferringAction to findSessionConn for WsStatusTransferring routing"
```

---

## Task 7: Pass `transferringAction` in `SendToSession` and `SendToSessionWithAck`

**Files:**

- Modify: `core/service/websocket/mediator/service/1.send_notification_to_session.go`
- Modify: `core/service/websocket/mediator/service/1.send_with_ack.go`

`SendToSession` needs to save the pre-wrapped message to MessageHub when the session is Transferring. `SendToSessionWithAck` does the same — the ACK will time out (session is reconnecting) but the message is buffered for delivery on reconnect.

- [ ] **Step 1: Update `SendToSession`**

In `core/service/websocket/mediator/service/1.send_notification_to_session.go`, add the `transferringAction` import and closure.

The file needs these imports: `fn "github.com/pipewave-dev/go-pkg/shared/utils/fn"` and `wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"`. Check if they're already imported; the current file imports `br` and `aerror`. Add the needed ones.

Replace the full file content:

```go
package mediatorsvc

import (
	"context"
	"log/slog"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	fn "github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (m *mediatorSvc) SendToSession(ctx context.Context, userID string, instanceID string, msgType string, payload []byte) aerror.AError {
	pl := br.SendToSessionParams{
		UserId:     userID,
		InstanceId: instanceID,
		MsgType:    msgType,
		Payload:    payload,
	}
	localAction := func() {
		m.broadcastHandler.SendToSession(ctx, pl)
	}
	targetContainerAction := func(containerIDs []string) {
		err := m.broadcast.SendToSession(ctx, containerIDs, pl).Publish()
		if err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast SendToSession",
				slog.String("userID", userID),
				slog.String("instanceID", instanceID),
				slog.Any("containerIDs", containerIDs),
				slog.Any("error", err))
		}
	}
	// When the session is in WsStatusTransferring, wrap the message and save to MessageHub
	// so the client receives it upon reconnect to any container.
	transferringAction := func() aerror.AError {
		id := fn.NewUUID()
		wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(), "", wsSv.MessageType(msgType), payload)
		return m.msgHubSvc.Save(ctx, userID, instanceID, wsRes)
	}

	findThenAction := &findSessionConn{
		ctx:                   ctx,
		userID:                userID,
		instanceID:            instanceID,
		localAction:           localAction,
		targetContainerAction: targetContainerAction,
		transferringAction:    transferringAction,
		callbackNotfound: func() {
			slog.WarnContext(ctx, "InstanceID not found when SendToSession",
				slog.String("userID", userID),
				slog.String("instanceID", instanceID))
		},
		c:              m.c,
		connections:    m.connections,
		activeConnRepo: m.activeConnRepo,
	}

	return findThenAction.findThenAction()
}
```

- [ ] **Step 2: Update `SendToSessionWithAck`**

In `core/service/websocket/mediator/service/1.send_with_ack.go`, update the `findSessionConn` struct literal inside `SendToSessionWithAck` to add `transferringAction`:

```go
// When the session is in WsStatusTransferring, buffer message to MessageHub.
// ACK will not arrive (session is reconnecting); caller receives found=false.
transferringAction := func() aerror.AError {
    id := fn.NewUUID()
    wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(), "", wsSv.MessageType(pl.MsgType), pl.Payload)
    return m.msgHubSvc.Save(ctx, userID, instanceID, wsRes)
}

findThenAction := &findSessionConn{
    ctx:                   ctx,
    userID:                userID,
    instanceID:            instanceID,
    localAction:           localAction,
    targetContainerAction: targetContainerAction,
    transferringAction:    transferringAction,
    callbackNotfound:      func() {},
    c:                     m.c,
    connections:           m.connections,
    activeConnRepo:        m.activeConnRepo,
}
```

Add imports for `wsSv` and `fn` to `1.send_with_ack.go`:

```go
import (
    "context"
    "log/slog"
    "time"

    voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
    br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
    wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
    "github.com/pipewave-dev/go-pkg/shared/aerror"
    fn "github.com/pipewave-dev/go-pkg/shared/utils/fn"
)
```

- [ ] **Step 3: Build**

```bash
go build ./core/service/websocket/mediator/service/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add core/service/websocket/mediator/service/1.send_notification_to_session.go \
        core/service/websocket/mediator/service/1.send_with_ack.go
git commit -m "feat(mediator): route Transferring sessions to MessageHub in SendToSession and SendToSessionWithAck"
```

---

## Task 8: Fix `onCloseRegister` — Skip DB Operations During Shutdown

**Files:**

- Modify: `core/service/websocket/mediator/delivery/0.new.go`

**Context:** When `IsShuttingDown()=true`, the `Shutdown()` method (Task 10) has already called `UpdateStatusTransferring` + `msgHubSvc.Register` for every connection BEFORE closing them. If `onClose` then calls `RemoveConnection`, it deletes the Transferring record that the client needs. Fix: when shutting down, skip all DB ops in `onClose`.

- [ ] **Step 1: Update `onCloseRegister`**

In `core/service/websocket/mediator/delivery/0.new.go`, replace the `onCloseRegister` method body:

```go
func (d *serverDelivery) onCloseRegister() {
	d.onCloseStuff.RegisterAll(func(auth voAuth.WebsocketAuth) {
		d.connectionMgr.RemoveConnection(auth)
		d.rateLimiter.Remove(auth)

		ctx := context.Background()

		// Anonymous sessions: always remove permanently (no reconnect buffering for anon).
		if auth.IsAnonymous() {
			if aErr := d.activeConnRepo.RemoveConnection(ctx, auth.UserID, auth.InstanceID); aErr != nil {
				slog.Error("onClose: failed to remove anonymous connection",
					slog.Any("auth", auth), slog.Any("error", aErr))
			}
			return
		}

		// Graceful shutdown path: Shutdown() already called UpdateStatusTransferring +
		// msgHubSvc.Register for this connection before closing. Skip all DB operations
		// to avoid overwriting the Transferring record.
		if d.shutdownSignal.IsShuttingDown() {
			return
		}

		// Normal temp-disconnect path: keep DB record + HolderID for cross-container routing.
		aErr := d.activeConnRepo.UpdateStatus(ctx, auth.UserID, auth.InstanceID, voWs.WsStatusTempDisconnected)
		if aErr != nil {
			slog.Error("onClose: UpdateStatus failed, falling back to RemoveConnection",
				slog.Any("auth", auth), slog.Any("error", aErr))
			_ = d.activeConnRepo.RemoveConnection(ctx, auth.UserID, auth.InstanceID)
			return
		}

		d.msgHubSvc.Register(auth.UserID, auth.InstanceID, func() {
			// ExpiredTimer fired — session never reconnected within TTL.
			if err := d.activeConnRepo.RemoveConnection(ctx, auth.UserID, auth.InstanceID); err != nil {
				slog.Error("onExpired: failed to remove ActiveConnection",
					slog.String("userID", auth.UserID),
					slog.String("instanceID", auth.InstanceID),
					slog.Any("error", err))
			}
		})
	})
}
```

- [ ] **Step 2: Build**

```bash
go build ./core/service/websocket/mediator/delivery/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add core/service/websocket/mediator/delivery/0.new.go
git commit -m "fix(delivery): skip DB operations in onClose during graceful shutdown"
```

---

## Task 9: Fix `onNewRegister` — Handle `WsStatusTransferring` + Drain Pattern

**Files:**

- Modify: `core/service/websocket/mediator/delivery/0.new.go`

**Context:** Two changes in `onNewRegister`:

1. Handle `WsStatusTransferring` (old container shutting down — no `ResumeSession` needed since HolderID is empty).
2. Apply `DrainableConn` pattern: `BeginDrain` before `connectionMgr.AddConnection`, send pending via `SendDirect`, then `EndDrain` — ensuring no concurrent send reaches the client before the pending messages.

- [ ] **Step 1: Update `onNewRegister`**

In `core/service/websocket/mediator/delivery/0.new.go`, replace the `onNewRegister` method body:

```go
func (d *serverDelivery) onNewRegister() {
	d.onNewStuff.Register(
		wsSv.OnNewWsKeyName("NewConnection"),
		func(connection wsSv.WebsocketConn) error {
			auth := connection.Auth()
			ctx := context.Background()

			// Close stale in-memory duplicate.
			if existingConn, ok := d.connectionMgr.GetConnection(auth); ok {
				slog.Warn("Duplicate connection detected, closing old connection")
				existingConn.Close()
				time.Sleep(time.Millisecond * 500)
			}

			// Check previous session state for reconnect handling.
			actConn, aErr := d.activeConnRepo.GetInstanceConnection(ctx, auth.UserID, auth.InstanceID)
			if aErr == nil && actConn != nil {
				switch actConn.Status {
				case voWs.WsStatusTempDisconnected:
					// Normal reconnect: signal old container to cancel its ExpiredTimer.
					if sigErr := d.wsService.ResumeSession(ctx, actConn.HolderID, auth.UserID, auth.InstanceID); sigErr != nil {
						slog.WarnContext(ctx, "onNew: ResumeSession publish failed; old ExpiredTimer will eventually fire",
							slog.String("holderID", actConn.HolderID),
							slog.String("userID", auth.UserID),
							slog.String("instanceID", auth.InstanceID),
							slog.Any("error", sigErr))
					}
				case voWs.WsStatusTransferring:
					// Container-shutdown reconnect: HolderID is empty, old container is shutting down.
					// No signal needed — AddConnection below will claim this session.
					slog.InfoContext(ctx, "onNew: reconnect after container shutdown (WsStatusTransferring)",
						slog.String("userID", auth.UserID),
						slog.String("instanceID", auth.InstanceID))
				}
			}

			// Upsert: updates HolderID to this container + resets Status to WsStatusConnected.
			if aErr = d.activeConnRepo.AddConnection(ctx, auth.UserID, auth.InstanceID, connection.CoreType()); aErr != nil {
				return aErr
			}

			// Begin drain BEFORE registering in ConnectionManager.
			// This blocks concurrent Send() calls (which acquire RLock) until drain is complete,
			// ensuring pending messages are delivered before any new messages.
			if dc, ok := connection.(wsSv.DrainableConn); ok {
				dc.BeginDrain()
				defer dc.EndDrain()
			}

			d.connectionMgr.AddConnection(connection)
			d.rateLimiter.New(auth)

			// Consume buffered pending messages and deliver them via SendDirect
			// (bypasses drainMu to avoid deadlock while WLock is held).
			msgs, consumeErr := d.msgHubSvc.Consume(ctx, auth.UserID, auth.InstanceID)
			if consumeErr != nil {
				slog.WarnContext(ctx, "onNew: failed to consume pending messages; session continues without them",
					slog.String("userID", auth.UserID),
					slog.String("instanceID", auth.InstanceID),
					slog.Any("error", consumeErr))
			}
			for _, msg := range msgs {
				if dc, ok := connection.(wsSv.DrainableConn); ok {
					if err := dc.SendDirect(msg); err != nil {
						slog.ErrorContext(ctx, "onNew: SendDirect failed for pending message",
							slog.String("userID", auth.UserID),
							slog.String("instanceID", auth.InstanceID),
							slog.Any("error", err))
					}
				} else {
					// Fallback: connection does not implement DrainableConn.
					if err := connection.Send(msg); err != nil {
						slog.ErrorContext(ctx, "onNew: Send failed for pending message",
							slog.String("userID", auth.UserID),
							slog.String("instanceID", auth.InstanceID),
							slog.Any("error", err))
					}
				}
			}
			// defer dc.EndDrain() fires here → blocked Send() goroutines proceed after pending messages.

			return nil
		})
}
```

- [ ] **Step 2: Build**

```bash
go build ./core/service/websocket/mediator/delivery/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add core/service/websocket/mediator/delivery/0.new.go
git commit -m "feat(delivery): handle WsStatusTransferring on reconnect and add drain ordering in onNewRegister"
```

---

## Task 10: Rewrite `mediatorSvc.Shutdown()`

**Files:**

- Modify: `core/service/websocket/mediator/service/3.shutdown.go`

**Context:** New shutdown sequence:

1. `MarkShuttingDown()` — signals `onClose` to skip DB ops.
2. `ackManager.Shutdown()` — unblock pending ACK waiters.
3. For each authenticated connection: `UpdateStatusTransferring` + `msgHubSvc.Register` (with `onExpired` cleanup).
4. Close all connections (last) — triggers `onClose` which now skips DB ops.

- [ ] **Step 1: Rewrite shutdown**

Replace the full content of `core/service/websocket/mediator/service/3.shutdown.go`:

```go
package mediatorsvc

import (
	"context"
	"log/slog"
)

// Shutdown performs graceful shutdown of the mediator service.
//
// Order of operations:
//  1. MarkShuttingDown — signals onClose to skip DB operations (we handle them here).
//  2. Cancel pending ACKs — unblocks goroutines waiting in WaitForAck.
//  3. For each authenticated connection:
//     a. UpdateStatusTransferring — atomically sets Status=Transferring, HolderID="".
//     b. Register in MessageHub — buffers messages for the session until TTL.
//  4. Close all connections — triggers onClose which skips DB ops (already handled above).
//
// Close is done last so that if DB updates are slow, the WebSocket remains open
// to receive and buffer messages until the record is committed.
func (m *mediatorSvc) Shutdown() {
	ctx := context.Background()

	// 1. Signal delivery layer: use shutdown path for all subsequent closes.
	m.shutdownSignal.MarkShuttingDown()

	// 2. Cancel all pending ACKs so goroutines blocked in WaitForAck are unblocked immediately.
	m.ackManager.Shutdown()

	// 3. Update all authenticated connections to WsStatusTransferring and register in MessageHub.
	allAuthConns := m.connections.GetAllAuthenticatedConn()
	for _, conn := range allAuthConns {
		auth := conn.Auth()
		if auth.IsAnonymous() {
			continue
		}

		// Atomically set Status=Transferring and clear HolderID so any container can claim on reconnect.
		if aErr := m.activeConnRepo.UpdateStatusTransferring(ctx, auth.UserID, auth.InstanceID); aErr != nil {
			slog.ErrorContext(ctx, "Shutdown: UpdateStatusTransferring failed — session may be lost",
				slog.String("userID", auth.UserID),
				slog.String("instanceID", auth.InstanceID),
				slog.Any("error", aErr))
			// Continue: best-effort. Other sessions should still be processed.
		}

		// Register in MessageHub so incoming messages are buffered until the client reconnects.
		// onExpired fires if TTL elapses without reconnect.
		// Guard: only remove the DB record if no other container has claimed the session
		// (HolderID still empty). If HolderID != "", the client reconnected elsewhere — skip removal.
		userID := auth.UserID
		instanceID := auth.InstanceID
		m.msgHubSvc.Register(userID, instanceID, func() {
			actConn, aErr := m.activeConnRepo.GetInstanceConnection(ctx, userID, instanceID)
			if aErr != nil || actConn == nil {
				return // Already removed or not found — nothing to do.
			}
			if actConn.HolderID != "" {
				// Another container claimed the session. Don't remove.
				return
			}
			if err := m.activeConnRepo.RemoveConnection(ctx, userID, instanceID); err != nil {
				slog.ErrorContext(ctx, "Shutdown.onExpired: failed to remove ActiveConnection",
					slog.String("userID", userID),
					slog.String("instanceID", instanceID),
					slog.Any("error", err))
			}
		})
	}

	// 4. Close all connections — onClose skips DB ops (MarkShuttingDown was called above).
	for _, conn := range m.connections.GetAllConnections() {
		conn.Close()
	}
}
```

- [ ] **Step 2: Full build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run all tests**

```bash
go test ./... -timeout 60s
```

Expected: all existing tests pass + `TestDrainOrdering` passes.

- [ ] **Step 4: Commit**

```bash
git add core/service/websocket/mediator/service/3.shutdown.go
git commit -m "feat(mediator): rewrite Shutdown to transfer sessions before closing connections"
```

---

## Self-Review Checklist

After implementation, verify:

- [ ] `go build ./...` — clean build
- [ ] `go test ./...` — all tests pass
- [ ] `UpdateStatusTransferring` implemented in both DynamoDB and Postgres impls
- [ ] `GobwasConnection` and `LongPollingConn` both satisfy `DrainableConn` (compile-time checks pass)
- [ ] `findSessionConn` handles `HolderID==""` via `transferringAction`
- [ ] `onCloseRegister`: anonymous→RemoveConnection, shutting_down→skip, normal→TempDisconnected path
- [ ] `onNewRegister`: `WsStatusTransferring` case logs and proceeds (no ResumeSession), drain lock wraps Consume+SendDirect
- [ ] `Shutdown()`: MarkShuttingDown → cancelACKs → UpdateStatusTransferring×N → Register×N → Close×N
