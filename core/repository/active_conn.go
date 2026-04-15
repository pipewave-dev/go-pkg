package repository

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

type ActiveConnStore interface {
	CountActiveConnections(ctx context.Context, userID string) (int, aerror.AError)

	// Count total active connections across all containers (include other containers when using loadbalancer)
	// Warn: this function will scan all items in the table, so it will be slow if the table is large (do not use it frequently)
	CountTotalActiveConnections(ctx context.Context) (int64, aerror.AError)
	AddConnection(ctx context.Context, userID string, instanceID string, connectionType voWs.WsCoreType) aerror.AError
	RemoveConnection(ctx context.Context, userID string, instanceID string) aerror.AError
	UpdateHeartBeat(ctx context.Context, userID string, instanceID string) aerror.AError

	// UpdateStatus updates only the WsStatus field of a connection record without changing HolderID or other fields.
	UpdateStatus(ctx context.Context, userID string, instanceID string, status voWs.WsStatus) aerror.AError

	// UpdateStatusTransferring atomically sets Status=WsStatusTransferring and clears HolderID="".
	// Used exclusively during graceful container shutdown so that any container can pick up the session on reconnect.
	UpdateStatusTransferring(ctx context.Context, userID string, instanceID string) aerror.AError

	// CountActiveConnectionsBatch returns connection counts for multiple users at once.
	CountActiveConnectionsBatch(ctx context.Context, userIDs []string) (map[string]int, aerror.AError)

	// GetActiveConnections returns all active connections for a user.
	GetActiveConnections(ctx context.Context, userID string) ([]entities.ActiveConnection, aerror.AError)

	// GetActiveConnectionsByUserIDs returns all active connections for multiple users.
	GetActiveConnectionsByUserIDs(ctx context.Context, userIDs []string) ([]entities.ActiveConnection, aerror.AError)

	// GetInstanceConnection returns an active connections for a session.
	GetInstanceConnection(ctx context.Context, userID string, instanceID string) (*entities.ActiveConnection, aerror.AError)

	CleanUpExpiredConnections(ctx context.Context) aerror.AError
}
