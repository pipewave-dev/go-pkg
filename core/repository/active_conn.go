package repository

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

type ActiveConnStore interface {
	CountActiveConnections(ctx context.Context, userID string) (int, aerror.AError)

	// Count total active connections across all containers (include other containers when using loadbalancer)
	// Warn: this function will scan all items in the table, so it will be slow if the table is large (do not use it frequently)
	CountTotalActiveConnections(ctx context.Context) (int64, aerror.AError)
	AddConnection(ctx context.Context, userID string, instanceID string) aerror.AError
	RemoveConnection(ctx context.Context, userID string, instanceID string) aerror.AError
	UpdateHeartBeat(ctx context.Context, userID string, instanceID string) aerror.AError

	// CountActiveConnectionsBatch returns connection counts for multiple users at once.
	CountActiveConnectionsBatch(ctx context.Context, userIDs []string) (map[string]int, aerror.AError)

	// GetActiveConnections returns all active connections for a user.
	GetActiveConnections(ctx context.Context, userID string) ([]entities.ActiveConnection, aerror.AError)
}
