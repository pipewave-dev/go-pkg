package entities

import (
	"time"

	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
)

type ActiveConnection struct {
	UserID    string // PartitionKey ~ contraint User.ID
	SessionID string // SortKey

	HolderID string // ContainerID that holds this connection, used for routing message via publish-subscribe system

	ConnectionType voWs.WsCoreType // ConnectionType enum, e.g. WebSocket, HTTP Long Polling, etc.
	Status         voWs.WsStatus

	ConnectedAt   time.Time
	LastHeartbeat time.Time
	TTL           time.Time
}
