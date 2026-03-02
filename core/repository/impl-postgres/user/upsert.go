package userRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpsert = "userRepo.Upsert"

func (r *userRepo) Upsert(ctx context.Context, userID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpsert)
	defer op.Finish(aErr)

	now := time.Now()

	query := `
		INSERT INTO users (id, last_heartbeat, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE
		SET last_heartbeat = $2
	`

	_, err := r.pool.Exec(ctx, query, userID, now, now)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
