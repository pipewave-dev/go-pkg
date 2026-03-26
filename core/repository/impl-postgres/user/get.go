package userRepo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGet = "userRepo.Get"

func (r *userRepo) Get(ctx context.Context, userID string) (user entities.User, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGet)
	defer op.Finish(aErr)

	query := `SELECT id, last_heartbeat, created_at FROM users WHERE id = $1`

	var result entities.User
	err := r.pool.QueryRow(ctx, query, userID).Scan(&result.ID, &result.LastHeartbeat, &result.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			aErr = aerror.New(ctx, aerror.RecordNotFound, err)
			return entities.User{}, aErr
		}
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return entities.User{}, aErr
	}

	return result, nil
}
