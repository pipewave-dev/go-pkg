package activeConnRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCleanUpExpiredConnections = "activeConnRepo.CleanUpExpiredConnections"

func (r *activeConnRepo) CleanUpExpiredConnections(ctx context.Context) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCleanUpExpiredConnections)
	defer op.Finish(aErr)

	query := `DELETE FROM active_connections WHERE ttl < NOW()`

	_, err := r.pool.Exec(ctx, query)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
