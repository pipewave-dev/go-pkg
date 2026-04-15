package activeConnRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/global/constants"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpdateHeartBeat = "activeConnRepo.UpdateHeartBeat"

func (r *activeConnRepo) UpdateHeartBeat(ctx context.Context, userID string, instanceID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpdateHeartBeat)
	defer op.Finish(aErr)

	now := time.Now()
	ttl := now.Add(2*constants.GlobalHeartbeatRateDuration + time.Second)

	query := `
		UPDATE active_connections
		SET last_heartbeat = $1, ttl = $2
		WHERE user_id = $3 AND instance_id = $4
	`

	_, err := r.pool.Exec(ctx, query, now, ttl, userID, instanceID)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
