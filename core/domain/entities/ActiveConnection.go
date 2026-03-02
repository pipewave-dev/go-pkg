package entities

import (
	"time"
)

type ActiveConnection struct {
	UserID    string // PartitionKey ~ contraint User.ID
	SessionID string // SortKey

	HolderID      string // Pod name holding this connection (env.PodName)
	LastHeartbeat time.Time
	TTL           time.Time
}
