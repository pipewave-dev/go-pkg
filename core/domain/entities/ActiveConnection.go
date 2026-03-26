package entities

import (
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
)

type ActiveConnection struct {
	UserID    string // PartitionKey ~ contraint User.ID
	SessionID string // SortKey

	HolderID string // ContainerID that holds this connection, used for routing message via publish-subscribe system

	ConnectionType voAuth.WsCoreType // ConnectionType enum, e.g. WebSocket, HTTP Long Polling, etc.

	ConnectedAt   time.Time
	LastHeartbeat time.Time
	TTL           time.Time
}
