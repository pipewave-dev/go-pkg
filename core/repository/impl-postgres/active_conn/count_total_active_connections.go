package activeConnRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCountTotalActiveConnections = "activeConnRepo.CountTotalActiveConnections"

func (r *activeConnRepo) CountTotalActiveConnections(ctx context.Context) (total int64, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCountTotalActiveConnections)
	defer op.Finish(aErr)

	cutoff := time.Now().Add(-2 * time.Minute)

	query := `
		SELECT COUNT(*) FROM active_connections
		WHERE last_heartbeat > $1
	`

	err := r.pool.QueryRow(ctx, query, cutoff).Scan(&total)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return 0, aErr
	}

	return total, nil
}
