package activeConnRepo

import (
	"context"

	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpdateStatusTransferring = "activeConnRepo.UpdateStatusTransferring"

func (r *activeConnRepo) UpdateStatusTransferring(ctx context.Context, userID string, instanceID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpdateStatusTransferring)
	defer op.Finish(aErr)

	query := `
		UPDATE active_connections
		SET status = $1, holder_id = ''
		WHERE user_id = $2 AND instance_id = $3
	`

	_, err := r.pool.Exec(ctx, query, voWs.WsStatusTransferring, userID, instanceID)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
