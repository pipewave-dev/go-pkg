package userRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpdateLastHeartbeat = "userRepo.UpdateLastHeartbeat"

func (r *userRepo) UpdateLastHeartbeat(ctx context.Context, userID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpdateLastHeartbeat)
	defer op.Finish(aErr)

	now := time.Now()

	query := `UPDATE users SET last_heartbeat = $1 WHERE id = $2`

	_, err := r.pool.Exec(ctx, query, now, userID)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
