# MessageHub Offline Buffering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Buffer WebSocket messages for temporarily disconnected sessions so they receive missed messages upon reconnection.

**Architecture:** When a session disconnects, a `MessageHub` record is created in the database as a signal that the session is waiting for reconnection. While the hub exists, messages targeting that session are saved as `PendingMessage` records. On reconnect, pending messages are consumed (sent to the client) and both the hub and messages are deleted. The hub has a configurable TTL (default 5s) after which it expires and messages are no longer buffered.

**Tech Stack:** Go, DynamoDB/PostgreSQL (repository interface pattern), Valkey (cache layer for hub existence checks), Wire (DI), msgpack (serialization)

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `core/service/websocket/message-hub/0_types.go` | `MessageHubService` interface definition |
| `core/service/websocket/message-hub/1_service.go` | Service implementation (business logic) |
| `core/service/websocket/message-hub/wire.go` | Wire provider set |
| `core/repository/message_hub.go` | `MessageHubStore` repository interface |
| `core/repository/pending_message.go` | `PendingMessageStore` repository interface |
| `core/domain/entities/message_hub.go` | `MessageHub` entity |
| `core/domain/entities/pending_message.go` | `PendingMessage` entity |

### Modified Files

| File | Change |
|------|--------|
| `core/repository/0.all_repo.go` | Add `MessageHubStore()` and `PendingMessageStore()` to `AllRepository` |
| `core/service/websocket/broadcast-msg-handler/0_main_handler.go` | Inject `MessageHubService` dependency |
| `core/service/websocket/broadcast-msg-handler/1_send_to_user.go` | Add fallback: save to hub if no local connection |
| `core/service/websocket/broadcast-msg-handler/1_send_to_session.go` | Add fallback: save to hub if no local connection |
| `core/service/websocket/broadcast-msg-handler/1_send_to_users.go` | Add fallback: save to hub per user/session |
| `core/service/websocket/broadcast-msg-handler/wire.go` | Update wire set for new dependency |
| `core/service/websocket/mediator/delivery/0.new.go` | Add hub creation on disconnect, hub consumption on connect |
| `provider/config-provider/0.1.env_types.go` | Add `MessageHub` config field to `globalEnvT` |
| `provider/config-provider/0.3.support_types.go` | Add `MessageHubT` config type |
| `config.yaml` | Add `MESSAGE_HUB` config section |

### Repository Implementation Files (one set per DB backend)

| File | Responsibility |
|------|---------------|
| `core/repository/impl-dynamodb/message_hub/0.1.new.go` | DynamoDB implementation constructor |
| `core/repository/impl-dynamodb/message_hub/create.go` | Create hub record |
| `core/repository/impl-dynamodb/message_hub/get.go` | Get hub record (filter expired) |
| `core/repository/impl-dynamodb/message_hub/delete.go` | Delete hub record |
| `core/repository/impl-dynamodb/pending_message/0.1.new.go` | DynamoDB implementation constructor |
| `core/repository/impl-dynamodb/pending_message/create.go` | Create pending message |
| `core/repository/impl-dynamodb/pending_message/get.go` | Get all pending messages (sorted by SendAt) |
| `core/repository/impl-dynamodb/pending_message/delete.go` | Batch delete pending messages |

---

## Task 1: Define Entities

**Files:**
- Create: `core/domain/entities/MessageHub.go`
- Create: `core/domain/entities/PendingMessage.go`

- [ ] **Step 1: Create MessageHub entity**

```go
// core/domain/entities/MessageHub.go
package entities

import "time"

type MessageHub struct {
	UserID    string
	SessionID string
	ExpiredAt time.Time
}

func (m *MessageHub) IsExpired() bool {
	return time.Now().After(m.ExpiredAt)
}
```

- [ ] **Step 2: Create PendingMessage entity**

```go
// core/domain/entities/PendingMessage.go
package entities

import "time"

type PendingMessage struct {
	HashKey   string    // userID + ":" + sessionID
	MessageID string    // UUID for idempotent writes across containers
	SendAt    time.Time
	MsgType   string
	Payload   []byte
}

func PendingMessageHashKey(userID, sessionID string) string {
	return userID + ":" + sessionID
}
```

- [ ] **Step 3: Commit**

```bash
git add core/domain/entities/MessageHub.go core/domain/entities/PendingMessage.go
git commit -m "feat: add MessageHub and PendingMessage entities"
```

---

## Task 2: Define Repository Interfaces

**Files:**
- Create: `core/repository/message_hub.go`
- Create: `core/repository/pending_message.go`
- Modify: `core/repository/0.all_repo.go`

- [ ] **Step 1: Create MessageHubStore interface**

```go
// core/repository/message_hub.go
package repository

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

type MessageHubStore interface {
	// Create creates a new message hub record. Returns error if already exists.
	Create(ctx context.Context, hub *entities.MessageHub) aerror.AError

	// Get retrieves a message hub by userID and sessionID.
	// Returns aerror.RecordNotFound if not found or expired.
	Get(ctx context.Context, userID string, sessionID string) (*entities.MessageHub, aerror.AError)

	// Delete removes a message hub record.
	Delete(ctx context.Context, userID string, sessionID string) aerror.AError

	// GetByUserID retrieves all active (non-expired) message hubs for a user.
	GetByUserID(ctx context.Context, userID string) ([]*entities.MessageHub, aerror.AError)
}
```

- [ ] **Step 2: Create PendingMessageStore interface**

```go
// core/repository/pending_message.go
package repository

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

type PendingMessageStore interface {
	// Create saves a pending message. Uses MessageID as sort key for idempotent writes.
	// Multiple containers calling Create with the same MessageID will result in only one record.
	Create(ctx context.Context, msg *entities.PendingMessage) aerror.AError

	// GetAll retrieves all pending messages for a hash key, ordered by SendAt ascending.
	GetAll(ctx context.Context, hashKey string) ([]*entities.PendingMessage, aerror.AError)

	// DeleteAll deletes all pending messages for a hash key (batch delete).
	DeleteAll(ctx context.Context, hashKey string) aerror.AError
}
```

- [ ] **Step 3: Add to AllRepository interface**

Modify `core/repository/0.all_repo.go`:

```go
type AllRepository interface {
	ActiveConnStore() ActiveConnStore
	User() User
	MessageHubStore() MessageHubStore
	PendingMessageStore() PendingMessageStore
}
```

- [ ] **Step 4: Update all AllRepository implementations to satisfy the new interface**

Two implementations exist that must be updated with stub methods (returning `nil`):

**DynamoDB** - `core/repository/impl-dynamodb/new_ddb_repo.go`:
```go
func (r *ddbRepo) MessageHubStore() repository.MessageHubStore       { return nil }
func (r *ddbRepo) PendingMessageStore() repository.PendingMessageStore { return nil }
```

**PostgreSQL** - `core/repository/impl-postgres/new_pg_repo.go`:
```go
func (r *pgRepo) MessageHubStore() repository.MessageHubStore       { return nil }
func (r *pgRepo) PendingMessageStore() repository.PendingMessageStore { return nil }
```

These stubs will be replaced with real implementations in Task 5.

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`
Expected: PASS (stubs return nil, interfaces satisfied)

- [ ] **Step 6: Commit**

```bash
git add core/repository/message_hub.go core/repository/pending_message.go core/repository/0.all_repo.go core/repository/impl-*
git commit -m "feat: add MessageHubStore and PendingMessageStore repository interfaces"
```

---

## Task 3: Add Configuration

**Files:**
- Modify: `provider/config-provider/0.1.env_types.go`
- Modify: `provider/config-provider/0.3.support_types.go`
- Modify: `config.yaml`

- [ ] **Step 1: Add MessageHubT config type**

Add to `provider/config-provider/0.3.support_types.go`:

```go
// MessageHubT contains configuration for offline message buffering
type MessageHubT struct {
	// TTLSeconds is the time-to-live for a message hub record in seconds.
	// After this duration, the hub expires and messages are no longer buffered.
	// Default: 5
	TTLSeconds int `koanf:"TTL_SECONDS"`
}

func (m MessageHubT) TTL() time.Duration {
	if m.TTLSeconds <= 0 {
		return 5 * time.Second
	}
	return time.Duration(m.TTLSeconds) * time.Second
}
```

Add `"time"` to imports if not present.

- [ ] **Step 2: Add MessageHub field to globalEnvT**

Add to `provider/config-provider/0.1.env_types.go`:

```go
MessageHub MessageHubT `koanf:"MESSAGE_HUB"`
```

Add this field after the `Postgres` field in `globalEnvT`.

- [ ] **Step 3: Add config section to config.yaml**

Add to `config.yaml`:

```yaml
MESSAGE_HUB:
  TTL_SECONDS: 5
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add provider/config-provider/0.1.env_types.go provider/config-provider/0.3.support_types.go config.yaml
git commit -m "feat: add MessageHub configuration with TTL setting"
```

---

## Task 4: Define MessageHubService Interface and Implementation

**Files:**
- Create: `core/service/websocket/message-hub/0_types.go`
- Create: `core/service/websocket/message-hub/1_service.go`
- Create: `core/service/websocket/message-hub/wire.go`

- [ ] **Step 1: Define MessageHubService interface**

```go
// core/service/websocket/message-hub/0_types.go
package messagehub

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

type MessageHubService interface {
	// CreateHub creates a new message hub for a disconnected session.
	CreateHub(ctx context.Context, userID string, sessionID string) aerror.AError

	// DeleteHub removes a message hub (called after consume or explicit cleanup).
	DeleteHub(ctx context.Context, userID string, sessionID string) aerror.AError

	// SaveMessage saves a message to the hub for later delivery.
	// messageID must be a stable UUID generated once per logical message (before pub/sub broadcast)
	// to ensure idempotent writes across multiple containers.
	// Returns false if no hub exists for the given userID+sessionID (meaning session has no pending hub).
	SaveMessage(ctx context.Context, userID string, sessionID string, messageID string, msgType string, payload []byte) (saved bool, aErr aerror.AError)

	// SaveMessageToUser saves a message to ALL active hubs for a user.
	// messageID must be a stable UUID generated once per logical message.
	// Returns the number of hubs the message was saved to.
	SaveMessageToUser(ctx context.Context, userID string, messageID string, msgType string, payload []byte) (savedCount int, aErr aerror.AError)

	// ConsumeMessages retrieves and deletes all pending messages for a session.
	// Returns empty slice if no hub or no messages exist.
	ConsumeMessages(ctx context.Context, userID string, sessionID string) ([]*entities.PendingMessage, aerror.AError)
}
```

- [ ] **Step 2: Implement MessageHubService**

```go
// core/service/websocket/message-hub/1_service.go
package messagehub

import (
	"context"
	"log/slog"
	"time"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

type messageHubService struct {
	c               configprovider.ConfigStore
	hubRepo         repository.MessageHubStore
	pendingMsgRepo  repository.PendingMessageStore
}

func New(
	c configprovider.ConfigStore,
	repo repository.AllRepository,
) MessageHubService {
	return &messageHubService{
		c:              c,
		hubRepo:        repo.MessageHubStore(),
		pendingMsgRepo: repo.PendingMessageStore(),
	}
}

func (s *messageHubService) CreateHub(ctx context.Context, userID string, sessionID string) aerror.AError {
	ttl := s.c.Env().MessageHub.TTL()
	hub := &entities.MessageHub{
		UserID:    userID,
		SessionID: sessionID,
		ExpiredAt: time.Now().Add(ttl),
	}
	return s.hubRepo.Create(ctx, hub)
}

func (s *messageHubService) DeleteHub(ctx context.Context, userID string, sessionID string) aerror.AError {
	hashKey := entities.PendingMessageHashKey(userID, sessionID)

	// Delete pending messages first, then hub
	if aErr := s.pendingMsgRepo.DeleteAll(ctx, hashKey); aErr != nil {
		slog.Error("Failed to delete pending messages",
			slog.String("userID", userID),
			slog.String("sessionID", sessionID),
			slog.Any("error", aErr))
	}

	return s.hubRepo.Delete(ctx, userID, sessionID)
}

func (s *messageHubService) SaveMessage(ctx context.Context, userID string, sessionID string, messageID string, msgType string, payload []byte) (bool, aerror.AError) {
	// Check if hub exists
	_, aErr := s.hubRepo.Get(ctx, userID, sessionID)
	if aErr != nil {
		// Hub not found or expired -> no buffering needed
		return false, nil
	}

	hashKey := entities.PendingMessageHashKey(userID, sessionID)
	msg := &entities.PendingMessage{
		HashKey:   hashKey,
		MessageID: messageID, // Stable ID ensures idempotent writes across containers
		SendAt:    time.Now(),
		MsgType:   msgType,
		Payload:   payload,
	}

	if aErr := s.pendingMsgRepo.Create(ctx, msg); aErr != nil {
		return false, aErr
	}

	return true, nil
}

func (s *messageHubService) SaveMessageToUser(ctx context.Context, userID string, messageID string, msgType string, payload []byte) (int, aerror.AError) {
	hubs, aErr := s.hubRepo.GetByUserID(ctx, userID)
	if aErr != nil {
		return 0, aErr
	}

	savedCount := 0
	for _, hub := range hubs {
		hashKey := entities.PendingMessageHashKey(hub.UserID, hub.SessionID)
		// Use messageID + sessionID to create unique but idempotent key per hub
		hubMsgID := messageID + ":" + hub.SessionID
		msg := &entities.PendingMessage{
			HashKey:   hashKey,
			MessageID: hubMsgID,
			SendAt:    time.Now(),
			MsgType:   msgType,
			Payload:   payload,
		}

		if aErr := s.pendingMsgRepo.Create(ctx, msg); aErr != nil {
			slog.Error("Failed to save pending message to hub",
				slog.String("userID", userID),
				slog.String("sessionID", hub.SessionID),
				slog.Any("error", aErr))
			continue
		}
		savedCount++
	}

	return savedCount, nil
}

func (s *messageHubService) ConsumeMessages(ctx context.Context, userID string, sessionID string) ([]*entities.PendingMessage, aerror.AError) {
	hashKey := entities.PendingMessageHashKey(userID, sessionID)

	messages, aErr := s.pendingMsgRepo.GetAll(ctx, hashKey)
	if aErr != nil {
		return nil, aErr
	}

	// Clean up: delete messages and hub
	if err := s.pendingMsgRepo.DeleteAll(ctx, hashKey); err != nil {
		slog.Error("Failed to delete consumed pending messages",
			slog.String("hashKey", hashKey),
			slog.Any("error", err))
	}

	if err := s.hubRepo.Delete(ctx, userID, sessionID); err != nil {
		slog.Error("Failed to delete consumed message hub",
			slog.String("userID", userID),
			slog.String("sessionID", sessionID),
			slog.Any("error", err))
	}

	return messages, nil
}
```

- [ ] **Step 3: Create wire provider**

```go
// core/service/websocket/message-hub/wire.go
package messagehub

import "github.com/google/wire"

var WireSet = wire.NewSet(New)
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/service/websocket/message-hub/
git commit -m "feat: implement MessageHubService for offline message buffering"
```

---

## Task 5: Implement DynamoDB Repository

**Files:**
- Create: `core/repository/impl-dynamodb/message_hub/0.1.new.go`
- Create: `core/repository/impl-dynamodb/message_hub/create.go`
- Create: `core/repository/impl-dynamodb/message_hub/get.go`
- Create: `core/repository/impl-dynamodb/message_hub/delete.go`
- Create: `core/repository/impl-dynamodb/pending_message/0.1.new.go`
- Create: `core/repository/impl-dynamodb/pending_message/create.go`
- Create: `core/repository/impl-dynamodb/pending_message/get.go`
- Create: `core/repository/impl-dynamodb/pending_message/delete.go`
- Modify: DynamoDB AllRepository implementation to wire new stores

**Important context:**
- Follow the same patterns as `core/repository/impl-dynamodb/active_conn/` for struct layout, constructor, DynamoDB client usage
- `MessageHub` table: partition key = `UserID`, sort key = `SessionID`, has `ExpiredAt` field, uses TTL attribute for auto-expiration
- `PendingMessage` table: partition key = `HashKey` (userID:sessionID), sort key = `MessageID` (UUID, ensures idempotent writes across containers), has `SendAt`, `MsgType` and `Payload` fields
- Add table name configs to `DynamoTables` struct and `config.yaml`

- [ ] **Step 1: Add table names to DynamoTables config**

Modify `provider/config-provider/0.3.support_types.go` `DynamoTables` struct:

```go
MessageHub     string `koanf:"MESSAGE_HUB"`
PendingMessage string `koanf:"PENDING_MESSAGE"`
```

Add to `config.yaml` under `DYNAMO_TABLES:`:

```yaml
  MESSAGE_HUB: "message_hub_dev"
  PENDING_MESSAGE: "pending_message_dev"
```

- [ ] **Step 2: Create MessageHub DynamoDB repository**

Create `core/repository/impl-dynamodb/message_hub/0.1.new.go`:

```go
package messagehub

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

type messageHubRepo struct {
	c    configprovider.ConfigStore
	ddbC *dynamodb.Client
	obs  observer.Observability
}

func New(
	c configprovider.ConfigStore,
	ddbC *dynamodb.Client,
	obs observer.Observability,
) repository.MessageHubStore {
	return &messageHubRepo{
		c:    c,
		ddbC: ddbC,
		obs:  obs,
	}
}
```

Follow the existing `active_conn` pattern for `create.go`, `get.go`, `delete.go` methods.

Key implementation notes:
- `Create`: Use `PutItem` with `UserID` as partition key, `SessionID` as sort key, `ExpiredAt` as TTL attribute
- `Get`: Use `GetItem`, check `ExpiredAt > now`, return `aerror.RecordNotFound` if expired or not found
- `Delete`: Use `DeleteItem`
- `GetByUserID`: Use `Query` with partition key = `UserID`, filter `ExpiredAt > now`

- [ ] **Step 3: Create PendingMessage DynamoDB repository**

Create `core/repository/impl-dynamodb/pending_message/0.1.new.go`:

```go
package pendingmessage

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

type pendingMessageRepo struct {
	c    configprovider.ConfigStore
	ddbC *dynamodb.Client
	obs  observer.Observability
}

func New(
	c configprovider.ConfigStore,
	ddbC *dynamodb.Client,
	obs observer.Observability,
) repository.PendingMessageStore {
	return &pendingMessageRepo{
		c:    c,
		ddbC: ddbC,
		obs:  obs,
	}
}
```

Key implementation notes:
- `Create`: Use `PutItem` with `HashKey` as partition key, `MessageID` as sort key. This makes writes **idempotent** — multiple containers writing the same messageID will overwrite the same record, preventing duplicates.
- `GetAll`: Use `Query` with partition key = `HashKey`. Results are sorted by `MessageID` (sort key). To get chronological order, sort by `SendAt` in application code after retrieval.
- `DeleteAll`: Use `Query` to get all items, then `BatchWriteItem` to delete (max 25 per batch)

- [ ] **Step 4: Wire new repos into AllRepository implementation**

Find the DynamoDB `AllRepository` implementation and add the new store methods:

```go
func (r *dynamoRepo) MessageHubStore() repository.MessageHubStore { return r.messageHubStore }
func (r *dynamoRepo) PendingMessageStore() repository.PendingMessageStore { return r.pendingMsgStore }
```

Initialize them in the constructor using the new sub-packages.

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add core/repository/impl-dynamodb/message_hub/ core/repository/impl-dynamodb/pending_message/ provider/config-provider/0.3.support_types.go config.yaml
git commit -m "feat: implement DynamoDB repositories for MessageHub and PendingMessage"
```

---

## Task 6: Integrate with OnClose (Create Hub on Disconnect)

**Files:**
- Modify: `core/service/websocket/mediator/delivery/0.new.go`

- [ ] **Step 1: Add MessageHubService dependency to serverDelivery**

Add to `serverDelivery` struct:

```go
messageHubSvc messagehub.MessageHubService
```

Add to `New()` constructor parameters:

```go
messageHubSvc messagehub.MessageHubService,
```

Assign in constructor body:

```go
ins.messageHubSvc = messageHubSvc
```

- [ ] **Step 2: Create hub in onCloseRegister**

Modify the `onCloseRegister()` method. Add hub creation **before** the existing cleanup logic:

```go
func (d *serverDelivery) onCloseRegister() {
	d.onCloseStuff.RegisterAll(func(auth voAuth.WebsocketAuth) {
		// Create message hub for disconnected session (only for authenticated users)
		if auth.UserID != "" {
			ctx := context.Background()
			if aErr := d.messageHubSvc.CreateHub(ctx, auth.UserID, auth.InstanceID); aErr != nil {
				slog.Error("Failed to create message hub on disconnect",
					slog.String("userID", auth.UserID),
					slog.String("sessionID", auth.InstanceID),
					slog.Any("error", aErr))
			}
		}

		// Existing cleanup logic (keep as-is)
		d.connectionMgr.RemoveConnection(auth)
		d.rateLimiter.Remove(auth)

		aErr := d.activeConnRepo.RemoveConnection(context.Background(), auth.UserID, auth.InstanceID)
		if aErr != nil {
			slog.Error("Failed to remove connection from DynamoDB",
				slog.Any("error", aErr),
				slog.Any("auth", auth))
		}
	})
}
```

- [ ] **Step 3: Update wire.go for delivery**

Modify `core/service/websocket/mediator/delivery/wire.go` to include the new dependency if needed.

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/service/websocket/mediator/delivery/
git commit -m "feat: create MessageHub on session disconnect"
```

---

## Task 7: Integrate with OnNew (Consume Hub on Reconnect)

**Files:**
- Modify: `core/service/websocket/mediator/delivery/0.new.go`

- [ ] **Step 1: Add hub consumption in onNewRegister**

Modify the `onNewRegister()` method. Add hub consumption **after** the connection is registered:

```go
func (d *serverDelivery) onNewRegister() {
	d.onNewStuff.Register(
		wsSv.OnNewWsKeyName("NewConnection"),
		func(connection wsSv.WebsocketConn) error {
			auth := connection.Auth()
			ctx := context.Background()

			// Check for duplicate connection in memory
			existingConn, ok := d.connectionMgr.GetConnection(connection.Auth())
			if ok {
				slog.Warn("Duplicate connection detected, closing old connection")
				existingConn.Close()
				time.Sleep(time.Millisecond * 500)
			}

			// Persist connection to DynamoDB
			aErr := d.activeConnRepo.AddConnection(ctx, auth.UserID, auth.InstanceID)
			if aErr != nil {
				return aErr
			}

			// Add to in-memory manager
			d.connectionMgr.AddConnection(connection)
			d.rateLimiter.New(connection.Auth())

			// Consume pending messages from hub (if any)
			if auth.UserID != "" {
				d.consumePendingMessages(connection)
			}

			return nil
		})
}

func (d *serverDelivery) consumePendingMessages(connection wsSv.WebsocketConn) {
	auth := connection.Auth()
	ctx := context.Background()

	messages, aErr := d.messageHubSvc.ConsumeMessages(ctx, auth.UserID, auth.InstanceID)
	if aErr != nil {
		slog.Error("Failed to consume pending messages",
			slog.String("userID", auth.UserID),
			slog.String("sessionID", auth.InstanceID),
			slog.Any("error", aErr))
		return
	}

	for _, msg := range messages {
		wsRes := wsSv.WrapperBytesToWebsocketResponse(
			fn.NewUUID().String(),
			"",
			wsSv.MessageType(msg.MsgType),
			msg.Payload,
		)
		connection.Send(wsRes)
	}

	if len(messages) > 0 {
		slog.Info("Delivered pending messages on reconnect",
			slog.String("userID", auth.UserID),
			slog.String("sessionID", auth.InstanceID),
			slog.Int("count", len(messages)))
	}
}
```

Add import `"github.com/pipewave-dev/go-pkg/shared/utils/fn"` to `delivery/0.new.go` if not already present.

- [ ] **Step 2: Verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add core/service/websocket/mediator/delivery/0.new.go
git commit -m "feat: consume pending messages from MessageHub on reconnect"
```

---

## Task 8: Integrate with Broadcast Handlers (Buffer on Send Failure)

**Files:**
- Modify: `core/service/websocket/broadcast-msg-handler/0_main_handler.go`
- Modify: `core/service/websocket/broadcast-msg-handler/1_send_to_user.go`
- Modify: `core/service/websocket/broadcast-msg-handler/1_send_to_session.go`
- Modify: `core/service/websocket/broadcast-msg-handler/1_send_to_users.go`
- Modify: `core/service/websocket/broadcast-msg-handler/wire.go`

- [ ] **Step 1: Add MessageHubService to broadcastMsgHandler**

Modify `core/service/websocket/broadcast-msg-handler/0_main_handler.go`:

```go
package broadcastmsghandler

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	messagehub "github.com/pipewave-dev/go-pkg/core/service/websocket/message-hub"
)

type broadcastMsgHandler struct {
	storeActiveWs repo.ActiveConnStore
	connections   wsSv.ConnectionManager
	messageHub    messagehub.MessageHubService
}

func New(
	repo repository.AllRepository,
	connections wsSv.ConnectionManager,
	messageHub messagehub.MessageHubService,
) broadcast.PubsubHandler {
	return &broadcastMsgHandler{
		storeActiveWs: repo.ActiveConnStore(),
		connections:   connections,
		messageHub:    messageHub,
	}
}
```

- [ ] **Step 2: Modify SendToSession to buffer when no connection**

**Deduplication strategy**: The `messageID` is generated once in the broadcast params (before pub/sub).
All containers receive the same `messageID`. PendingMessage uses `MessageID` as sort key in DynamoDB,
so multiple containers writing the same message produce a single record (PutItem is idempotent on same key).

Modify `core/service/websocket/broadcast-msg-handler/1_send_to_session.go`:

```go
func (h *broadcastMsgHandler) SendToSession(ctx context.Context, payload broadcast.SendToSessionParams) {
	auth := voAuth.UserWebsocketAuth(
		payload.UserId,
		payload.InstanceId,
	)

	conn, ok := h.connections.GetConnection(auth)
	if !ok {
		// No local connection -> try to save to hub (idempotent via messageID)
		if payload.UserId != "" {
			saved, aErr := h.messageHub.SaveMessage(ctx, payload.UserId, payload.InstanceId, payload.MessageId, payload.MsgType, payload.Payload)
			if aErr != nil {
				slog.Error("Failed to save message to hub",
					slog.String("userID", payload.UserId),
					slog.String("sessionID", payload.InstanceId),
					slog.Any("error", aErr))
			}
			if saved {
				slog.Debug("Message buffered in hub",
					slog.String("userID", payload.UserId),
					slog.String("sessionID", payload.InstanceId))
			}
		}
		return
	}

	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(), "",
		wsSv.MessageType(payload.MsgType), payload.Payload)
	conn.Send(wsRes)
}
```

Add `"log/slog"` to imports.

- [ ] **Step 3: Modify SendToUser to buffer for disconnected sessions**

Modify `core/service/websocket/broadcast-msg-handler/1_send_to_user.go`:

```go
func (h *broadcastMsgHandler) SendToUser(ctx context.Context, payload broadcast.SendToUserParams) {
	connections := h.connections.GetAllUserConn(payload.UserId)

	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(),
		"",
		wsSv.MessageType(payload.MsgType),
		payload.Payload)

	for _, conn := range connections {
		conn.Send(wsRes)
	}

	// Save to any active hubs for this user (disconnected sessions).
	// Uses messageID for idempotent writes — safe to call from multiple containers.
	if payload.UserId != "" {
		savedCount, aErr := h.messageHub.SaveMessageToUser(ctx, payload.UserId, payload.MessageId, payload.MsgType, payload.Payload)
		if aErr != nil {
			slog.Error("Failed to save message to user hubs",
				slog.String("userID", payload.UserId),
				slog.Any("error", aErr))
		}
		if savedCount > 0 {
			slog.Debug("Message buffered in user hubs",
				slog.String("userID", payload.UserId),
				slog.Int("hubCount", savedCount))
		}
	}
}
```

Add `"log/slog"` to imports.

- [ ] **Step 4: Modify SendToUsers similarly**

Modify `core/service/websocket/broadcast-msg-handler/1_send_to_users.go`:

```go
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

		// Save to any active hubs (idempotent via messageID)
		if userID != "" {
			_, aErr := h.messageHub.SaveMessageToUser(ctx, userID, payload.MessageId, payload.MsgType, payload.Payload)
			if aErr != nil {
				slog.Error("Failed to save message to user hubs",
					slog.String("userID", userID),
					slog.Any("error", aErr))
			}
		}
	}
}
```

Add `"log/slog"` to imports.

- [ ] **Step 5: Add MessageId field to broadcast param types**

Modify `core/service/websocket/broadcast/1.1.params_type.go` — add `MessageId string` field to:
- `SendToUserParams`
- `SendToSessionParams`
- `SendToUsersParams`

The `MessageId` must be generated once by the caller (in `mediator/service/`) before publishing to pub/sub, ensuring all containers receive the same ID.

- [ ] **Step 6: Update mediator service to generate MessageId before broadcast**

Modify `core/service/websocket/mediator/service/1.send_notification_to_user.go` (and similar send methods) to generate a UUID and pass it as `MessageId` in the broadcast params:

```go
// In SendToUser:
msgID := fn.NewUUID().String()
// Pass msgID in the broadcast params
```

Apply the same pattern to `SendToSession`, `SendToUsers`, `SendToSessionWithAck`, `SendToUserWithAck`.

- [ ] **Step 7: Update wire.go**

Modify `core/service/websocket/broadcast-msg-handler/wire.go` to include the new dependency.

- [ ] **Step 8: Verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add core/service/websocket/broadcast-msg-handler/ core/service/websocket/broadcast/1.1.params_type.go core/service/websocket/mediator/service/
git commit -m "feat: buffer messages in MessageHub when session has no active connection"
```

---

## Task 9: Wire Everything Together

**Files:**
- Modify: `gen/wire/default.go`

- [ ] **Step 1: Add MessageHubService to DefaultWireSet**

Modify `gen/wire/default.go` — add import and wire set:

```go
// Add to imports:
message_hub "github.com/pipewave-dev/go-pkg/core/service/websocket/message-hub"

// Add to DefaultWireSet:
message_hub.WireSet,
```

- [ ] **Step 2: Run wire generation**

Run: `go generate ./...` or the project's wire generation command.

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add gen/wire/default.go
git commit -m "feat: wire MessageHubService into application DI"
```

---

## Task 10: Add Cache Layer for Hub Existence Checks (Performance)

**Files:**
- Modify: `core/service/websocket/message-hub/1_service.go`

This task adds a Valkey/Redis cache in front of the `hubRepo.Get()` call in `SaveMessage()` to avoid hitting the database on every message send.

- [ ] **Step 1: Add cache to messageHubService**

The cache strategy:
- On `CreateHub`: set a cache key `messagehub:{userID}:{sessionID}` with TTL = hub TTL
- On `SaveMessage`/`SaveMessageToUser`: check cache first before hitting DB
- On `DeleteHub`/`ConsumeMessages`: delete cache key

Implementation depends on the project's existing cache/Valkey client pattern. Check how Valkey is used elsewhere in the project (note: use keyword `cacheprovider.CacheThis`).

```go
// Add to messageHubService struct:
// cache valkey.Client (or whatever the project's cache interface is)

// In SaveMessage, before hubRepo.Get():
// Check cache key "messagehub:{userID}:{sessionID}"
// If exists in cache -> hub exists, proceed to save message
// If not in cache -> hub doesn't exist, return false
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add core/service/websocket/message-hub/
git commit -m "feat: add Valkey cache layer for MessageHub existence checks"
```

---

## Task 11: Manual Integration Test

- [ ] **Step 1: Start the development server**

Run the server locally with DynamoDB Local and Valkey running.

- [ ] **Step 2: Test happy path**

1. Connect WebSocket client as User-X Session-A
2. Send a message to User-X -> verify Session-A receives it
3. Disconnect Session-A
4. Send message M-1 to User-X
5. Reconnect Session-A within 5s
6. Verify Session-A receives M-1 on reconnect

- [ ] **Step 3: Test TTL expiry**

1. Connect and disconnect Session-A
2. Wait > 5s (TTL expires)
3. Send message M-2 to User-X
4. Reconnect Session-A
5. Verify M-2 is NOT received (hub expired)

- [ ] **Step 4: Test multi-session**

1. Connect Session-A and Session-B for User-X
2. Disconnect Session-A (Session-B stays connected)
3. Send message to User-X
4. Verify Session-B receives it immediately
5. Reconnect Session-A
6. Verify Session-A receives buffered message

---

## Summary of Message Flow After Implementation

```
SendToUser(userID, msg):
  1. Mediator service generates messageID (UUID)
  2. pub/sub broadcast to all containers (messageID included in params)
  3. Each container's handler:
     a. connectionMgr.GetAllUserConn(userID) -> send to local connections
     b. messageHub.SaveMessageToUser(userID, messageID, msg) -> save to ALL active hubs
        (idempotent: same messageID from multiple containers = single record)
  4. On reconnect:
     a. onNewStuff callback fires
     b. consumePendingMessages() retrieves and sends buffered messages
     c. Hub and pending messages are cleaned up

SendToSession(userID, sessionID, msg):
  1. Mediator service generates messageID (UUID)
  2. pub/sub broadcast to all containers (messageID included in params)
  3. Each container's handler:
     a. connectionMgr.GetConnection(auth) -> send if local
     b. If no local connection: messageHub.SaveMessage(userID, sessionID, messageID, msg)
        (idempotent: same messageID from multiple containers = single record)
  4. On reconnect: same as above

Disconnect event:
  1. onCloseStuff.Do(auth) fires
  2. CreateHub(userID, sessionID) with TTL
  3. Existing cleanup (remove from connectionMgr, activeConnRepo, rateLimiter)

Connect event:
  1. onNewStuff.Do(conn) fires
  2. Existing setup (add to connectionMgr, activeConnRepo, rateLimiter)
  3. ConsumeMessages(userID, sessionID) -> send buffered messages
```

## Known Behaviors

- **Shutdown**: When the server shuts down gracefully, `onCloseStuff` fires for all sessions, creating MessageHub records. These expire after TTL (default 5s). During this window, messages may be buffered unnecessarily. This is acceptable behavior.
- **Race conditions**: A small window exists between disconnect event and hub creation where messages can be lost. Similarly, between message broadcast and disconnect, messages may be sent to a connection that's about to close. These are accepted trade-offs with logging for monitoring.
