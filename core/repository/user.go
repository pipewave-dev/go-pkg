package repository

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

type User interface {
	Upsert(ctx context.Context, userID string) aerror.AError
	Get(ctx context.Context, userID string) (entities.User, aerror.AError)
	UpdateLastHeartbeat(ctx context.Context, userID string) aerror.AError
}
