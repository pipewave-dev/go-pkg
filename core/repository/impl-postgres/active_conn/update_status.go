package activeConnRepo

import (
	"context"

	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpdateStatus = "activeConnRepo.UpdateStatus"

func (r *activeConnRepo) UpdateStatus(ctx context.Context, userID string, instanceID string, status voWs.WsStatus) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpdateStatus)
	defer op.Finish(aErr)

	query := `
		UPDATE active_connections
		SET status = $1
		WHERE user_id = $2 AND session_id = $3
	`

	_, err := r.pool.Exec(ctx, query, status, userID, instanceID)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
