package activeConnRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnRemoveConnection = "activeConnRepo.RemoveConnection"

func (r *activeConnRepo) RemoveConnection(ctx context.Context, userID string, instanceID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnRemoveConnection)
	defer op.Finish(aErr)

	query := `DELETE FROM active_connections WHERE user_id = $1 AND instance_id = $2`

	_, err := r.pool.Exec(ctx, query, userID, instanceID)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
