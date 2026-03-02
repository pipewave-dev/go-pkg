package repository

import (
	"context"

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

	// TODO: should have a function to clean up expired connections (if heartbeat is not updated for a long time)
}
