# Pipewave SDK — 7 New Features Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 7 features to Pipewave SDK: DisconnectSession/User, SendToUsers, Broadcast, Presence, OTEL Metrics, Connection Metadata, Message ACK.

**Architecture:** Each feature follows the existing Clean Architecture pattern: add to interface → add pub/sub channel + params → implement service → implement handler → wire up. The `broadcast/create_msg.go` is code-generated from `1.0.msg_enum.go` via `gen-template-for-enum`, so new channels require re-running the generator.

**Tech Stack:** Go, gobwas/ws, Valkey/Redis pub/sub, DynamoDB/Postgres, msgpack, OpenTelemetry, Google Wire

---

## File Structure Overview

### New files to create:
- `core/service/websocket/mediator/service/1.disconnect_session.go` — DisconnectSession service
- `core/service/websocket/mediator/service/1.disconnect_user.go` — DisconnectUser service
- `core/service/websocket/mediator/service/1.send_notification_to_users.go` — SendToUsers service
- `core/service/websocket/mediator/service/1.broadcast.go` — Broadcast service
- `core/service/websocket/mediator/service/1.check_online_multiple.go` — CheckOnlineMultiple service
- `core/service/websocket/mediator/service/1.get_user_sessions.go` — GetUserSessions service
- `core/service/websocket/mediator/service/1.send_with_ack.go` — SendWithAck service
- `core/service/websocket/broadcast-msg-handler/1_disconnect_session.go` — DisconnectSession handler
- `core/service/websocket/broadcast-msg-handler/1_disconnect_user.go` — DisconnectUser handler
- `core/service/websocket/broadcast-msg-handler/1_send_to_users.go` — SendToUsers handler
- `core/service/websocket/broadcast-msg-handler/1_broadcast.go` — Broadcast handler
- `core/service/websocket/ack-manager/ack_manager.go` — ACK pending map manager
- `core/repository/impl-dynamodb/active_conn/get_active_connections.go` — GetActiveConnections DynamoDB
- `core/repository/impl-dynamodb/active_conn/count_active_connections_batch.go` — CountActiveConnectionsBatch DynamoDB
- `core/repository/impl-postgres/active_conn/get_active_connections.go` — GetActiveConnections Postgres
- `core/repository/impl-postgres/active_conn/count_active_connections_batch.go` — CountActiveConnectionsBatch Postgres
- `pkg/metrics/metrics.go` — OTEL metrics provider

### Files to modify:
- `core/delivery/module.go` — Add new methods to `ExportedServices` and `ModuleDelivery`
- `core/service/websocket/0.ws_service.go` — Add new methods to `WsService`
- `core/service/websocket/0.message_type.go` — Add `AckId` field to `WebsocketResponse`
- `core/service/websocket/broadcast/1.0.msg_enum.go` — Add new channels
- `core/service/websocket/broadcast/1.1.params_type.go` — Add new params types
- `core/service/websocket/broadcast/create_msg.go` — Re-generate (auto)
- `core/service/websocket/3_connection_manager.go` — Add `GetAllAuthenticatedConn()` method
- `core/service/websocket/connection-manager/connection_mamanger.go` — Implement `GetAllAuthenticatedConn()`
- `core/delivery/module/3.get_service.go` — Wire new ExportedServices methods
- `core/delivery/module/0.0.new.go` — Add MetricsHandler
- `core/repository/active_conn.go` — Add new methods to `ActiveConnStore`
- `core/domain/entities/ActiveConnection.go` — Add `ConnectedAt` field
- `core/domain/value-object/auth/ws_auth.go` — Add `Metadata` field
- `provider/config-provider/0.4.fns.go` — Update `InspectToken` signature
- `core/service/websocket/mediator/delivery/1.issue_tmp_token.go` — Handle metadata from InspectToken
- `core/service/websocket/mediator/delivery/3.long_polling.go` — Handle metadata from InspectToken
- `core/service/websocket/mediator/delivery/4.long_polling_send.go` — Handle metadata from InspectToken
- `core/service/websocket/client-msg-handler/0_main_handler.go` — Handle `__ack__` message type
- `core/service/websocket/mediator/service/0.new.go` — Add ackManager dependency
- `app/wire_gen.go` — Re-generate (auto via `go generate`)

---

## Chunk 1: Phase 1 — Core Missing Features

### Task 1: Add DisconnectSession / DisconnectUser

**Files:**
- Modify: `core/service/websocket/broadcast/1.0.msg_enum.go`
- Modify: `core/service/websocket/broadcast/1.1.params_type.go`
- Modify: `core/service/websocket/0.ws_service.go`
- Modify: `core/delivery/module.go`
- Create: `core/service/websocket/broadcast-msg-handler/1_disconnect_session.go`
- Create: `core/service/websocket/broadcast-msg-handler/1_disconnect_user.go`
- Create: `core/service/websocket/mediator/service/1.disconnect_session.go`
- Create: `core/service/websocket/mediator/service/1.disconnect_user.go`
- Modify: `core/delivery/module/3.get_service.go`

- [ ] **Step 1: Add new pub/sub channels to enum**

In `core/service/websocket/broadcast/1.0.msg_enum.go`, add:
```go
// DisconnectSessionParams
channelDisconnectSession pubsubChannel = "DisconnectSession"
// DisconnectUserParams
channelDisconnectUser pubsubChannel = "DisconnectUser"
```

- [ ] **Step 2: Add new params types**

In `core/service/websocket/broadcast/1.1.params_type.go`, add:
```go
type DisconnectSessionParams struct {
	UserId     string
	InstanceId string
}

func (p *DisconnectSessionParams) Marshal() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("DisconnectSessionParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *DisconnectSessionParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}

type DisconnectUserParams struct {
	UserId string
}

func (p *DisconnectUserParams) Marshal() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("DisconnectUserParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *DisconnectUserParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}
```

- [ ] **Step 3: Re-generate `create_msg.go`**

Run: `go generate ./core/service/websocket/broadcast/...`

This will regenerate `create_msg.go` from the template, adding `DisconnectSession` and `DisconnectUser` methods to both `MsgCreator` and `PubsubHandler` interfaces, plus subscriber registrations.

If the generator is not available or doesn't work, manually add the methods following the existing pattern in `create_msg.go`.

- [ ] **Step 4: Add methods to WsService interface**

In `core/service/websocket/0.ws_service.go`, add to `WsService` interface:
```go
DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError
DisconnectUser(ctx context.Context, userID string) aerror.AError
```

- [ ] **Step 5: Add methods to ExportedServices interface**

In `core/delivery/module.go`, add to `ExportedServices` interface:
```go
// DisconnectSession force disconnects a specific session
DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError

// DisconnectUser force disconnects all sessions of a user
DisconnectUser(ctx context.Context, userID string) aerror.AError
```

- [ ] **Step 6: Implement broadcast handler for DisconnectSession**

Create `core/service/websocket/broadcast-msg-handler/1_disconnect_session.go`:
```go
package broadcastmsghandler

import (
	"context"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
)

func (h *broadcastMsgHandler) DisconnectSession(ctx context.Context, payload broadcast.DisconnectSessionParams) {
	auth := voAuth.UserWebsocketAuth(payload.UserId, payload.InstanceId)

	conn, ok := h.connections.GetConnection(auth)
	if !ok {
		return
	}

	conn.Close()
}
```

- [ ] **Step 7: Implement broadcast handler for DisconnectUser**

Create `core/service/websocket/broadcast-msg-handler/1_disconnect_user.go`:
```go
package broadcastmsghandler

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
)

func (h *broadcastMsgHandler) DisconnectUser(ctx context.Context, payload broadcast.DisconnectUserParams) {
	connections := h.connections.GetAllUserConn(payload.UserId)
	for _, conn := range connections {
		conn.Close()
	}
}
```

- [ ] **Step 8: Implement mediator service for DisconnectSession**

Create `core/service/websocket/mediator/service/1.disconnect_session.go`:
```go
package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError {
	pbPayload := br.DisconnectSessionParams{
		UserId:     userID,
		InstanceId: instanceID,
	}
	return m.broadcast.DisconnectSession(ctx, pbPayload).Publish()
}
```

- [ ] **Step 9: Implement mediator service for DisconnectUser**

Create `core/service/websocket/mediator/service/1.disconnect_user.go`:
```go
package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) DisconnectUser(ctx context.Context, userID string) aerror.AError {
	pbPayload := br.DisconnectUserParams{
		UserId: userID,
	}
	return m.broadcast.DisconnectUser(ctx, pbPayload).Publish()
}
```

- [ ] **Step 10: Wire up in getServices**

In `core/delivery/module/3.get_service.go`, add delegate methods:
```go
func (g *getServices) DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError {
	return g.wsService.DisconnectSession(ctx, userID, instanceID)
}

func (g *getServices) DisconnectUser(ctx context.Context, userID string) aerror.AError {
	return g.wsService.DisconnectUser(ctx, userID)
}
```

- [ ] **Step 11: Verify compilation**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 12: Commit**

```bash
git add core/service/websocket/broadcast/1.0.msg_enum.go \
  core/service/websocket/broadcast/1.1.params_type.go \
  core/service/websocket/broadcast/create_msg.go \
  core/service/websocket/0.ws_service.go \
  core/delivery/module.go \
  core/service/websocket/broadcast-msg-handler/1_disconnect_session.go \
  core/service/websocket/broadcast-msg-handler/1_disconnect_user.go \
  core/service/websocket/mediator/service/1.disconnect_session.go \
  core/service/websocket/mediator/service/1.disconnect_user.go \
  core/delivery/module/3.get_service.go
git commit -m "feat: add DisconnectSession and DisconnectUser APIs"
```

---

### Task 2: Add SendToUsers (Batch Send)

**Files:**
- Modify: `core/service/websocket/broadcast/1.0.msg_enum.go` (already modified in Task 1)
- Modify: `core/service/websocket/broadcast/1.1.params_type.go` (already modified in Task 1)
- Create: `core/service/websocket/broadcast-msg-handler/1_send_to_users.go`
- Create: `core/service/websocket/mediator/service/1.send_notification_to_users.go`
- Modify: `core/service/websocket/0.ws_service.go` (already modified in Task 1)
- Modify: `core/delivery/module.go` (already modified in Task 1)
- Modify: `core/delivery/module/3.get_service.go` (already modified in Task 1)

- [ ] **Step 1: Add pub/sub channel**

In `core/service/websocket/broadcast/1.0.msg_enum.go`, add:
```go
// SendToUsersParams
channelSendToUsers pubsubChannel = "SendToUsers"
```

- [ ] **Step 2: Add params type**

In `core/service/websocket/broadcast/1.1.params_type.go`, add:
```go
type SendToUsersParams struct {
	UserIds []string
	MsgType string
	Payload []byte
}

func (p *SendToUsersParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("SendToUsersParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *SendToUsersParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}
```

- [ ] **Step 3: Re-generate `create_msg.go`**

Run: `go generate ./core/service/websocket/broadcast/...`

- [ ] **Step 4: Add to WsService and ExportedServices**

In `core/service/websocket/0.ws_service.go`, add:
```go
SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError
```

In `core/delivery/module.go`, add:
```go
// SendToUsers broadcasts to multiple users in a single publish
SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError
```

- [ ] **Step 5: Implement broadcast handler**

Create `core/service/websocket/broadcast-msg-handler/1_send_to_users.go`:
```go
package broadcastmsghandler

import (
	"context"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) SendToUsers(ctx context.Context, payload broadcast.SendToUsersParams) {
	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(),
		"",
		wsSv.MessageType(payload.MsgType),
		payload.Payload)

	for _, userID := range payload.UserIds {
		connections := h.connections.GetAllUserConn(userID)
		for _, conn := range connections {
			conn.Send(wsRes)
		}
	}
}
```

- [ ] **Step 6: Implement mediator service**

Create `core/service/websocket/mediator/service/1.send_notification_to_users.go`:
```go
package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError {
	pbPayload := br.SendToUsersParams{
		UserIds: userIDs,
		MsgType: msgType,
		Payload: payload,
	}
	return m.broadcast.SendToUsers(ctx, pbPayload).Publish()
}
```

- [ ] **Step 7: Wire up in getServices**

In `core/delivery/module/3.get_service.go`, add:
```go
func (g *getServices) SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError {
	return g.wsService.SendToUsers(ctx, userIDs, msgType, payload)
}
```

- [ ] **Step 8: Verify compilation**

Run: `go build ./...`

- [ ] **Step 9: Commit**

```bash
git add core/service/websocket/broadcast/1.0.msg_enum.go \
  core/service/websocket/broadcast/1.1.params_type.go \
  core/service/websocket/broadcast/create_msg.go \
  core/service/websocket/0.ws_service.go \
  core/delivery/module.go \
  core/service/websocket/broadcast-msg-handler/1_send_to_users.go \
  core/service/websocket/mediator/service/1.send_notification_to_users.go \
  core/delivery/module/3.get_service.go
git commit -m "feat: add SendToUsers batch send API"
```

---

### Task 3: Add Broadcast (Global with Target Filter)

**Files:**
- Modify: `core/service/websocket/broadcast/1.0.msg_enum.go`
- Modify: `core/service/websocket/broadcast/1.1.params_type.go`
- Modify: `core/service/websocket/3_connection_manager.go`
- Modify: `core/service/websocket/connection-manager/connection_mamanger.go`
- Create: `core/service/websocket/broadcast-msg-handler/1_broadcast.go`
- Create: `core/service/websocket/mediator/service/1.broadcast.go`
- Modify: `core/service/websocket/0.ws_service.go`
- Modify: `core/delivery/module.go`
- Modify: `core/delivery/module/3.get_service.go`

- [ ] **Step 1: Add BroadcastTarget type to delivery module**

In `core/delivery/module.go`, add before `ExportedServices` interface:
```go
type BroadcastTarget int

const (
	BroadcastAll      BroadcastTarget = iota // All connections (authenticated + anonymous)
	BroadcastAuthOnly                        // Only authenticated users
	BroadcastAnonOnly                        // Only anonymous connections
)
```

- [ ] **Step 2: Add pub/sub channel and params**

In `core/service/websocket/broadcast/1.0.msg_enum.go`, add:
```go
// BroadcastParams
channelBroadcast pubsubChannel = "Broadcast"
```

In `core/service/websocket/broadcast/1.1.params_type.go`, add:
```go
type BroadcastParams struct {
	Target  int
	MsgType string
	Payload []byte
}

func (p *BroadcastParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("BroadcastParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *BroadcastParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}
```

- [ ] **Step 3: Re-generate `create_msg.go`**

Run: `go generate ./core/service/websocket/broadcast/...`

- [ ] **Step 4: Add `GetAllAuthenticatedConn` to ConnectionManager**

In `core/service/websocket/3_connection_manager.go`, add to interface:
```go
GetAllAuthenticatedConn() []WebsocketConn
```

In `core/service/websocket/connection-manager/connection_mamanger.go`, add:
```go
func (m *connectionMap) GetAllAuthenticatedConn() []wsSv.WebsocketConn {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var connections []wsSv.WebsocketConn
	for _, userClients := range m.userConn {
		for _, conn := range userClients {
			connections = append(connections, conn)
		}
	}
	return connections
}
```

- [ ] **Step 5: Add to WsService and ExportedServices**

In `core/service/websocket/0.ws_service.go`, add:
```go
Broadcast(ctx context.Context, target int, msgType string, payload []byte) aerror.AError
```

In `core/delivery/module.go`, add:
```go
// Broadcast sends to all connected clients based on target filter
Broadcast(ctx context.Context, target BroadcastTarget, msgType string, payload []byte) aerror.AError
```

- [ ] **Step 6: Implement broadcast handler**

Create `core/service/websocket/broadcast-msg-handler/1_broadcast.go`:
```go
package broadcastmsghandler

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) Broadcast(ctx context.Context, payload broadcast.BroadcastParams) {
	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(),
		"",
		wsSv.MessageType(payload.MsgType),
		payload.Payload)

	var connections []wsSv.WebsocketConn

	switch delivery.BroadcastTarget(payload.Target) {
	case delivery.BroadcastAll:
		connections = h.connections.GetAllConnections()
	case delivery.BroadcastAuthOnly:
		connections = h.connections.GetAllAuthenticatedConn()
	case delivery.BroadcastAnonOnly:
		connections = h.connections.GetAllAnonymousConn()
	default:
		return
	}

	for _, conn := range connections {
		conn.Send(wsRes)
	}
}
```

- [ ] **Step 7: Implement mediator service**

Create `core/service/websocket/mediator/service/1.broadcast.go`:
```go
package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) Broadcast(ctx context.Context, target int, msgType string, payload []byte) aerror.AError {
	pbPayload := br.BroadcastParams{
		Target:  target,
		MsgType: msgType,
		Payload: payload,
	}
	return m.broadcast.Broadcast(ctx, pbPayload).Publish()
}
```

- [ ] **Step 8: Wire up in getServices**

In `core/delivery/module/3.get_service.go`, add:
```go
func (g *getServices) Broadcast(ctx context.Context, target delivery.BroadcastTarget, msgType string, payload []byte) aerror.AError {
	return g.wsService.Broadcast(ctx, int(target), msgType, payload)
}
```

- [ ] **Step 9: Verify compilation**

Run: `go build ./...`

- [ ] **Step 10: Commit**

```bash
git add core/delivery/module.go \
  core/service/websocket/broadcast/1.0.msg_enum.go \
  core/service/websocket/broadcast/1.1.params_type.go \
  core/service/websocket/broadcast/create_msg.go \
  core/service/websocket/3_connection_manager.go \
  core/service/websocket/connection-manager/connection_mamanger.go \
  core/service/websocket/0.ws_service.go \
  core/service/websocket/broadcast-msg-handler/1_broadcast.go \
  core/service/websocket/mediator/service/1.broadcast.go \
  core/delivery/module/3.get_service.go
git commit -m "feat: add Broadcast API with target filter (All/AuthOnly/AnonOnly)"
```

---

## Chunk 2: Phase 2 — Enhanced Observability & Presence

### Task 4: Presence System (CheckOnlineMultiple + GetUserSessions)

**Files:**
- Modify: `core/repository/active_conn.go`
- Modify: `core/domain/entities/ActiveConnection.go`
- Create: `core/repository/impl-dynamodb/active_conn/count_active_connections_batch.go`
- Create: `core/repository/impl-dynamodb/active_conn/get_active_connections.go`
- Create: `core/repository/impl-postgres/active_conn/count_active_connections_batch.go`
- Create: `core/repository/impl-postgres/active_conn/get_active_connections.go`
- Modify: `core/service/websocket/0.ws_service.go`
- Modify: `core/delivery/module.go`
- Create: `core/service/websocket/mediator/service/1.check_online_multiple.go`
- Create: `core/service/websocket/mediator/service/1.get_user_sessions.go`
- Modify: `core/delivery/module/3.get_service.go`

- [ ] **Step 1: Add ConnectedAt to ActiveConnection entity**

In `core/domain/entities/ActiveConnection.go`, add field:
```go
type ActiveConnection struct {
	UserID    string // PartitionKey ~ constraint User.ID
	SessionID string // SortKey

	HolderID      string // Pod name holding this connection (env.PodName)
	ConnectedAt   time.Time
	LastHeartbeat time.Time
	TTL           time.Time
}
```

Also update `core/repository/impl-dynamodb/active_conn/exprbuilder/1.ddb.go` — add `ConnectedAt` field to `ddbActiveConnection`, `toDynamoDBItem`, `toEntity`, and add `FieldConnectedAt` constant.

Update `core/repository/impl-dynamodb/active_conn/exprbuilder/0.2.creator.go` — set `ConnectedAt` when creating.

- [ ] **Step 2: Add new methods to ActiveConnStore interface**

In `core/repository/active_conn.go`, add:
```go
CountActiveConnectionsBatch(ctx context.Context, userIDs []string) (map[string]int, aerror.AError)
GetActiveConnections(ctx context.Context, userID string) ([]entities.ActiveConnection, aerror.AError)
```

Import: `"github.com/pipewave-dev/go-pkg/core/domain/entities"`

- [ ] **Step 3: Implement DynamoDB CountActiveConnectionsBatch**

Create `core/repository/impl-dynamodb/active_conn/count_active_connections_batch.go`:
```go
package activeConnRepo

import (
	"context"
	"time"

	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCountActiveConnectionsBatch = "activeConnRepo.CountActiveConnectionsBatch"

func (r *activeConnRepo) CountActiveConnectionsBatch(ctx context.Context, userIDs []string) (result map[string]int, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCountActiveConnectionsBatch)
	defer op.Finish(aErr)

	result = make(map[string]int, len(userIDs))
	querier := activeConnExp.ActiveConnectionQuerier{ConfigStore: r.c}

	for _, userID := range userIDs {
		count, err := querier.CountActive(ctx, r.ddbC, activeConnExp.CountActiveParams{
			UserID:         userID,
			CutOffDuration: -2 * time.Minute,
		})
		if err != nil {
			return nil, err
		}
		result[userID] = count
	}

	return result, nil
}
```

- [ ] **Step 4: Implement DynamoDB GetActiveConnections**

Create `core/repository/impl-dynamodb/active_conn/get_active_connections.go`:
```go
package activeConnRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGetActiveConnections = "activeConnRepo.GetActiveConnections"

func (r *activeConnRepo) GetActiveConnections(ctx context.Context, userID string) (result []entities.ActiveConnection, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGetActiveConnections)
	defer op.Finish(aErr)

	querier := activeConnExp.ActiveConnectionQuerier{ConfigStore: r.c}
	items, err := querier.QueryByUserID(ctx, r.ddbC, userID)
	if err != nil {
		return nil, err
	}

	return items, nil
}
```

Also add `QueryByUserID` method to `exprbuilder/0.4.querier.go` that queries all active connections for a userID and returns `[]entities.ActiveConnection`.

- [ ] **Step 5: Implement Postgres CountActiveConnectionsBatch**

Create `core/repository/impl-postgres/active_conn/count_active_connections_batch.go`:
```go
package activeConnRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCountActiveConnectionsBatch = "activeConnRepo.CountActiveConnectionsBatch"

func (r *activeConnRepo) CountActiveConnectionsBatch(ctx context.Context, userIDs []string) (result map[string]int, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCountActiveConnectionsBatch)
	defer op.Finish(aErr)

	result = make(map[string]int, len(userIDs))
	cutoff := time.Now().Add(-2 * time.Minute)

	query := `
		SELECT user_id, COUNT(*) as cnt FROM active_connections
		WHERE user_id = ANY($1) AND last_heartbeat > $2
		GROUP BY user_id
	`

	rows, err := r.pool.Query(ctx, query, userIDs, cutoff)
	if err != nil {
		return nil, aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID string
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			return nil, aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		}
		result[userID] = count
	}

	// Fill in zeros for users not found
	for _, uid := range userIDs {
		if _, ok := result[uid]; !ok {
			result[uid] = 0
		}
	}

	return result, nil
}
```

- [ ] **Step 6: Implement Postgres GetActiveConnections**

Create `core/repository/impl-postgres/active_conn/get_active_connections.go`:
```go
package activeConnRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGetActiveConnections = "activeConnRepo.GetActiveConnections"

func (r *activeConnRepo) GetActiveConnections(ctx context.Context, userID string) (result []entities.ActiveConnection, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGetActiveConnections)
	defer op.Finish(aErr)

	cutoff := time.Now().Add(-2 * time.Minute)

	query := `
		SELECT user_id, session_id, holder_id, connected_at, last_heartbeat, ttl
		FROM active_connections
		WHERE user_id = $1 AND last_heartbeat > $2
	`

	rows, err := r.pool.Query(ctx, query, userID, cutoff)
	if err != nil {
		return nil, aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
	}
	defer rows.Close()

	for rows.Next() {
		var ac entities.ActiveConnection
		if err := rows.Scan(&ac.UserID, &ac.SessionID, &ac.HolderID, &ac.ConnectedAt, &ac.LastHeartbeat, &ac.TTL); err != nil {
			return nil, aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		}
		result = append(result, ac)
	}

	return result, nil
}
```

- [ ] **Step 7: Add SessionInfo type and methods to interfaces**

In `core/delivery/module.go`, add type and interface methods:
```go
type SessionInfo struct {
	InstanceID  string
	ConnectedAt time.Time
	IsAnonymous bool
}
```

Add to `ExportedServices`:
```go
// CheckOnlineMultiple checks online status of multiple users at once
CheckOnlineMultiple(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError)

// GetUserSessions returns detailed session info for a user
GetUserSessions(ctx context.Context, userID string) ([]SessionInfo, aerror.AError)
```

Add to `WsService` in `core/service/websocket/0.ws_service.go`:
```go
CheckOnlineMultiple(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError)
GetUserSessions(ctx context.Context, userID string) ([]delivery.SessionInfo, aerror.AError)
```

- [ ] **Step 8: Implement mediator service methods**

Create `core/service/websocket/mediator/service/1.check_online_multiple.go`:
```go
package mediatorsvc

import (
	"context"

	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) CheckOnlineMultiple(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError) {
	counts, aErr := m.activeConnRepo.CountActiveConnectionsBatch(ctx, userIDs)
	if aErr != nil {
		return nil, aErr
	}

	result := make(map[string]bool, len(counts))
	for userID, count := range counts {
		result[userID] = count > 0
	}
	return result, nil
}
```

Create `core/service/websocket/mediator/service/1.get_user_sessions.go`:
```go
package mediatorsvc

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) GetUserSessions(ctx context.Context, userID string) ([]delivery.SessionInfo, aerror.AError) {
	connections, aErr := m.activeConnRepo.GetActiveConnections(ctx, userID)
	if aErr != nil {
		return nil, aErr
	}

	sessions := make([]delivery.SessionInfo, 0, len(connections))
	for _, conn := range connections {
		sessions = append(sessions, delivery.SessionInfo{
			InstanceID:  conn.SessionID,
			ConnectedAt: conn.ConnectedAt,
			IsAnonymous: conn.UserID == "",
		})
	}
	return sessions, nil
}
```

- [ ] **Step 9: Wire up in getServices**

In `core/delivery/module/3.get_service.go`, add:
```go
func (g *getServices) CheckOnlineMultiple(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError) {
	return g.wsService.CheckOnlineMultiple(ctx, userIDs)
}

func (g *getServices) GetUserSessions(ctx context.Context, userID string) ([]delivery.SessionInfo, aerror.AError) {
	return g.wsService.GetUserSessions(ctx, userID)
}
```

- [ ] **Step 10: Verify compilation**

Run: `go build ./...`

- [ ] **Step 11: Commit**

```bash
git add core/domain/entities/ActiveConnection.go \
  core/repository/active_conn.go \
  core/repository/impl-dynamodb/active_conn/ \
  core/repository/impl-postgres/active_conn/ \
  core/service/websocket/0.ws_service.go \
  core/delivery/module.go \
  core/service/websocket/mediator/service/1.check_online_multiple.go \
  core/service/websocket/mediator/service/1.get_user_sessions.go \
  core/delivery/module/3.get_service.go
git commit -m "feat: add Presence system (CheckOnlineMultiple, GetUserSessions)"
```

---

### Task 5: Connection Metadata

**Files:**
- Modify: `core/domain/value-object/auth/ws_auth.go`
- Modify: `provider/config-provider/0.4.fns.go`
- Modify: `core/service/websocket/mediator/delivery/1.issue_tmp_token.go`
- Modify: `core/service/websocket/mediator/delivery/3.long_polling.go`
- Modify: `core/service/websocket/mediator/delivery/4.long_polling_send.go`

- [ ] **Step 1: Add Metadata to WebsocketAuth**

In `core/domain/value-object/auth/ws_auth.go`, update struct:
```go
type WebsocketAuth struct {
	UserID     string
	InstanceID string
	Metadata   map[string]string
}
```

Update factory functions to accept metadata:
```go
func UserWebsocketAuth(userID string, instanceID string) WebsocketAuth {
	// keep unchanged — metadata is optional
}

func UserWebsocketAuthWithMetadata(userID string, instanceID string, metadata map[string]string) WebsocketAuth {
	if userID == "" || instanceID == "" {
		panic("voAuth: UserWebsocketAuthWithMetadata called with empty userID or instanceID")
	}
	return WebsocketAuth{
		UserID:     userID,
		InstanceID: instanceID,
		Metadata:   metadata,
	}
}

func AnonymousUserWebsocketAuthWithMetadata(instanceID string, metadata map[string]string) WebsocketAuth {
	if instanceID == "" {
		panic("voAuth: AnonymousUserWebsocketAuthWithMetadata called with empty instanceID")
	}
	return WebsocketAuth{
		InstanceID: instanceID,
		Metadata:   metadata,
	}
}
```

- [ ] **Step 2: Update InspectToken signature**

In `provider/config-provider/0.4.fns.go`, update:
```go
InspectToken func(ctx context.Context, token string) (username string, IsAnonymous bool, metadata map[string]string, err error)
```

- [ ] **Step 3: Update all InspectToken call sites**

In `core/service/websocket/mediator/delivery/1.issue_tmp_token.go`, update line 32:
```go
username, isAnonymous, metadata, err := fns.InspectToken(r.Context(), authHeader)
```
And update wsAuth creation to use `WithMetadata` variants:
```go
if isAnonymous {
	wsAuth = voAuth.AnonymousUserWebsocketAuthWithMetadata(instanceHeader, metadata)
} else {
	wsAuth = voAuth.UserWebsocketAuthWithMetadata(username, instanceHeader, metadata)
}
```

Apply same pattern to `3.long_polling.go` and `4.long_polling_send.go`.

- [ ] **Step 4: Update pipewave.go public API**

The `FunctionStore = configprovider.Fns` type alias will pick up the change automatically. Update `README.md` examples to show new signature. (This is a **breaking change** for users — document in CHANGELOG.)

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`

- [ ] **Step 6: Commit**

```bash
git add core/domain/value-object/auth/ws_auth.go \
  provider/config-provider/0.4.fns.go \
  core/service/websocket/mediator/delivery/1.issue_tmp_token.go \
  core/service/websocket/mediator/delivery/3.long_polling.go \
  core/service/websocket/mediator/delivery/4.long_polling_send.go
git commit -m "feat: add Connection Metadata via InspectToken and WebsocketAuth"
```

---

### Task 6: OTEL Metrics Export

**Files:**
- Create: `pkg/metrics/metrics.go`
- Modify: `core/delivery/module.go`
- Modify: `core/delivery/module/0.0.new.go`
- Modify: `core/service/websocket/broadcast-msg-handler/0_main_handler.go` (instrument)
- Modify: `app/wire_gen.go` (re-generate)

- [ ] **Step 1: Create OTEL metrics provider**

Create `pkg/metrics/metrics.go`:
```go
package metrics

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	promclient "github.com/prometheus/client_golang/prometheus/promhttp"
)

type PipewaveMetrics struct {
	ActiveConnections   metric.Int64UpDownCounter
	MessagesSent        metric.Int64Counter
	MessagesReceived    metric.Int64Counter
	ConnDurationSeconds metric.Float64Histogram
	WorkerPoolUtil      metric.Float64Gauge
	PubsubMessages      metric.Int64Counter

	handler http.Handler
}

func New() *PipewaveMetrics {
	exporter, err := prometheus.New()
	if err != nil {
		panic(err)
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	meter := provider.Meter("pipewave")

	m := &PipewaveMetrics{
		handler: promclient.Handler(),
	}

	m.ActiveConnections, _ = meter.Int64UpDownCounter("pipewave_active_connections",
		metric.WithDescription("Number of active WebSocket connections"))
	m.MessagesSent, _ = meter.Int64Counter("pipewave_messages_sent_total",
		metric.WithDescription("Total messages sent"))
	m.MessagesReceived, _ = meter.Int64Counter("pipewave_messages_received_total",
		metric.WithDescription("Total messages received from clients"))
	m.ConnDurationSeconds, _ = meter.Float64Histogram("pipewave_connection_duration_seconds",
		metric.WithDescription("Duration of WebSocket connections"))
	m.WorkerPoolUtil, _ = meter.Float64Gauge("pipewave_worker_pool_utilization",
		metric.WithDescription("Worker pool utilization ratio"))
	m.PubsubMessages, _ = meter.Int64Counter("pipewave_pubsub_messages_total",
		metric.WithDescription("Total pub/sub messages published"))

	return m
}

func (m *PipewaveMetrics) Handler() http.Handler {
	return m.handler
}

func (m *PipewaveMetrics) RecordConnectionOpen(ctx context.Context, connType string) {
	m.ActiveConnections.Add(ctx, 1, metric.WithAttributes(
		attribute.String("type", connType),
	))
}

func (m *PipewaveMetrics) RecordConnectionClose(ctx context.Context, connType string) {
	m.ActiveConnections.Add(ctx, -1, metric.WithAttributes(
		attribute.String("type", connType),
	))
}

func (m *PipewaveMetrics) RecordMessageSent(ctx context.Context, target string) {
	m.MessagesSent.Add(ctx, 1, metric.WithAttributes(
		attribute.String("target", target),
	))
}

func (m *PipewaveMetrics) RecordMessageReceived(ctx context.Context) {
	m.MessagesReceived.Add(ctx, 1)
}
```

Note: Add proper imports for `go.opentelemetry.io/otel/attribute`.

- [ ] **Step 2: Add MetricsHandler to ModuleDelivery**

In `core/delivery/module.go`, add to `ModuleDelivery`:
```go
MetricsHandler() http.Handler
```

- [ ] **Step 3: Integrate metrics into moduleDelivery**

In `core/delivery/module/0.0.new.go`:
- Add `metrics *metrics.PipewaveMetrics` field to `moduleDelivery`
- Initialize in `New()` constructor
- Add `MetricsHandler()` method

- [ ] **Step 4: Instrument broadcast handlers with metrics**

Add metric recording calls in the broadcast msg handlers (SendToUser, SendToSession, etc.) and in the client msg handler for received messages.

This can be done incrementally — start with basic counters, add more instrumentation later.

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`

- [ ] **Step 6: Commit**

```bash
git add pkg/metrics/ \
  core/delivery/module.go \
  core/delivery/module/0.0.new.go
git commit -m "feat: add OTEL metrics export with Prometheus handler"
```

---

## Chunk 3: Phase 3 — Advanced Features

### Task 7: Message Acknowledgment

**Files:**
- Create: `core/service/websocket/ack-manager/ack_manager.go`
- Modify: `core/service/websocket/0.message_type.go`
- Modify: `core/service/websocket/0.ws_service.go`
- Modify: `core/delivery/module.go`
- Modify: `core/service/websocket/client-msg-handler/0_main_handler.go`
- Create: `core/service/websocket/mediator/service/1.send_with_ack.go`
- Modify: `core/service/websocket/mediator/service/0.new.go`
- Modify: `core/delivery/module/3.get_service.go`

- [ ] **Step 1: Create ACK manager**

Create `core/service/websocket/ack-manager/ack_manager.go`:
```go
package ackmanager

import (
	"sync"
	"time"

	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

type AckManager struct {
	mu      sync.RWMutex
	pending map[string]chan struct{}
}

func New() *AckManager {
	return &AckManager{
		pending: make(map[string]chan struct{}),
	}
}

// CreateAck creates a new ack ID and returns it with a channel that will be closed when ACK is received.
func (a *AckManager) CreateAck() (ackID string, ch chan struct{}) {
	ackID = "ack_" + fn.NewNanoID(18)
	ch = make(chan struct{})

	a.mu.Lock()
	a.pending[ackID] = ch
	a.mu.Unlock()

	return ackID, ch
}

// ResolveAck resolves a pending ACK. Returns true if the ackID was found and resolved.
func (a *AckManager) ResolveAck(ackID string) bool {
	a.mu.Lock()
	ch, ok := a.pending[ackID]
	if ok {
		delete(a.pending, ackID)
	}
	a.mu.Unlock()

	if ok {
		close(ch)
		return true
	}
	return false
}

// WaitForAck waits for an ACK with a timeout. Returns true if ACK was received.
func (a *AckManager) WaitForAck(ackID string, ch chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ch:
		return true
	case <-timer.C:
		// Timeout — clean up
		a.mu.Lock()
		delete(a.pending, ackID)
		a.mu.Unlock()
		return false
	}
}
```

- [ ] **Step 2: Add AckId field to WebsocketResponse**

In `core/service/websocket/0.message_type.go`, update:
```go
type WebsocketResponse struct {
	Id           string      `msgpack:"i,omitempty"`
	ResponseToId string      `msgpack:"r,omitempty"`
	MsgType      MessageType `msgpack:"t"`
	Error        string      `msgpack:"e,omitempty"`
	Binary       []byte      `msgpack:"b,omitempty"`
	AckId        string      `msgpack:"a,omitempty"` // NEW: for ACK mechanism
}
```

Add ACK message type constant:
```go
var MessageTypeAck = MessageType("__ack__")
```

- [ ] **Step 3: Add ACK methods to interfaces**

In `core/service/websocket/0.ws_service.go`, add:
```go
SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)
SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)
```

In `core/delivery/module.go`, add:
```go
// SendToSessionWithAck sends to a specific session and waits for client acknowledgment
SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)

// SendToUserWithAck sends to a user and waits for client acknowledgment
SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)
```

- [ ] **Step 4: Handle __ack__ in client message handler**

In `core/service/websocket/client-msg-handler/0_main_handler.go`:
- Add `ackManager *ackmanager.AckManager` field to `clientMsgHandler`
- Update `New()` to accept and store ackManager
- Add case in `handleMessage` switch for `MessageTypeAck`:

```go
case wsSv.MessageTypeAck:
	// Extract ackId from binary payload
	var ackMsg struct {
		AckId string `msgpack:"ackId"`
	}
	if err := msgpack.Unmarshal(msg.Binary, &ackMsg); err == nil && ackMsg.AckId != "" {
		h.ackManager.ResolveAck(ackMsg.AckId)
	}
	return // No response needed
```

- [ ] **Step 5: Implement mediator service ACK methods**

Create `core/service/websocket/mediator/service/1.send_with_ack.go`:
```go
package mediatorsvc

import (
	"context"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (m *mediatorSvc) SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	auth := voAuth.UserWebsocketAuth(userID, instanceID)
	conn, ok := m.connections.GetConnection(auth)
	if !ok {
		return false, nil
	}

	ackID, ch := m.ackManager.CreateAck()

	id := fn.NewUUID()
	wsRes := &wsSv.WebsocketResponse{
		Id:      id.String(),
		MsgType: wsSv.MessageType(msgType),
		Binary:  payload,
		AckId:   ackID,
	}
	conn.Send(wsRes.Marshall())

	return m.ackManager.WaitForAck(ackID, ch, timeout), nil
}

func (m *mediatorSvc) SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	connections := m.connections.GetAllUserConn(userID)
	if len(connections) == 0 {
		return false, nil
	}

	ackID, ch := m.ackManager.CreateAck()

	id := fn.NewUUID()
	wsRes := &wsSv.WebsocketResponse{
		Id:      id.String(),
		MsgType: wsSv.MessageType(msgType),
		Binary:  payload,
		AckId:   ackID,
	}
	data := wsRes.Marshall()

	for _, conn := range connections {
		conn.Send(data)
	}

	return m.ackManager.WaitForAck(ackID, ch, timeout), nil
}
```

- [ ] **Step 6: Add ackManager to mediatorSvc**

In `core/service/websocket/mediator/service/0.new.go`:
- Add `ackManager *ackmanager.AckManager` field to `mediatorSvc`
- Initialize in `New()`:
```go
ackMgr := ackmanager.New()
ins := &mediatorSvc{
	// ... existing fields
	ackManager: ackMgr,
}
```

- [ ] **Step 7: Wire up in getServices**

In `core/delivery/module/3.get_service.go`, add:
```go
func (g *getServices) SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	return g.wsService.SendToSessionWithAck(ctx, userID, instanceID, msgType, payload, timeout)
}

func (g *getServices) SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	return g.wsService.SendToUserWithAck(ctx, userID, msgType, payload, timeout)
}
```

- [ ] **Step 8: Wire ackManager through to clientMsgHandler**

Update Wire DI chain to pass ackManager from mediatorSvc to clientMsgHandler, or create a shared singleton ackManager. The simplest approach: create ackManager as a singleton in the wire setup and inject into both.

- [ ] **Step 9: Re-generate Wire**

Run: `go generate ./app/...` or `wire ./app/...`

- [ ] **Step 10: Verify compilation**

Run: `go build ./...`

- [ ] **Step 11: Commit**

```bash
git add core/service/websocket/ack-manager/ \
  core/service/websocket/0.message_type.go \
  core/service/websocket/0.ws_service.go \
  core/delivery/module.go \
  core/service/websocket/client-msg-handler/0_main_handler.go \
  core/service/websocket/mediator/service/1.send_with_ack.go \
  core/service/websocket/mediator/service/0.new.go \
  core/delivery/module/3.get_service.go \
  app/wire_gen.go
git commit -m "feat: add Message Acknowledgment (SendToSessionWithAck, SendToUserWithAck)"
```

---

## Post-Implementation

- [ ] **Update README.md** with new API examples
- [ ] **Update FRONTEND_SDK_TODO.md** if ACK protocol details changed during implementation
- [ ] **Run full test suite**: `go test ./...`
- [ ] **Final commit**: documentation updates
