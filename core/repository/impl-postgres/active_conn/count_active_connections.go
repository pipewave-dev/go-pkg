package activeConnRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCountActiveConnections = "activeConnRepo.CountActiveConnections"

func (r *activeConnRepo) CountActiveConnections(ctx context.Context, userID string) (count int, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCountActiveConnections)
	defer op.Finish(aErr)

	cutoff := time.Now().Add(-r.c.Env().ActConn.HeartbeatCutoff)

	query := `
		SELECT COUNT(*) FROM active_connections
		WHERE user_id = $1 AND last_heartbeat > $2
	`

	err := r.pool.QueryRow(ctx, query, userID, cutoff).Scan(&count)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return 0, aErr
	}

	return count, nil
}
