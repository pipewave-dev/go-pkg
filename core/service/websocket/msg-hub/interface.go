package msghub

import (
	"context"
)

// MessageHubSvc buffers pre-wrapped WebSocket response bytes for temporarily disconnected sessions.
// It holds an in-memory registry (this container only) and delegates DB persistence to PendingMessageRepo.
type MessageHubSvc interface {
	// This registry is only relevant for temp-disconnected sessions, and manages in memory.
	//
	// Register starts an ExpiredTimer; onExpired is called when TTL elapses without reconnect.
	Register(userID, instanceID string, onExpired func())
	// Deregister cancels the timer and removes the session from the registry.
	Deregister(userID, instanceID string)
	// IsRegistered reports whether this container holds a temp-disconnect entry for the session.
	IsRegistered(userID, instanceID string) bool
	// GetSessions returns all temp-disconnected instanceIDs for userID on this container.
	GetSessions(userID string) []string

	// This DB persistence is relevant for all sessions, but only used when a temp-disconnect occurs. Can call from any container.
	// Save stores pre-wrapped WebSocket response bytes for a temp-disconnected session.
	Save(ctx context.Context, userID, instanceID string, wrappedMsg []byte) error
	// Consume runs GetAll → DeleteAll → return. Prefers duplicate delivery over message loss.
	Consume(ctx context.Context, userID, instanceID string) ([][]byte, error)
	// DeleteAllPendingMessage deletes all pending messages for a session, used when a temp-disconnected session reconnects and we want to clear buffered messages that are no longer relevant.
	DeleteAllPendingMessage(ctx context.Context, userID, instanceID string)

	Shutdown() // clean up resources, e.g. stop background goroutines for expired timers

	// TODO:
	// CleanUpExpiredPendingMessage(ctx context.Context) error
}
