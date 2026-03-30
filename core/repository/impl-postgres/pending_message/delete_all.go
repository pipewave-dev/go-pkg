package pendingMessageRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnDeleteAll = "pendingMessageRepo.DeleteAll"

func (r *pendingMessageRepo) DeleteAll(ctx context.Context, userID, instanceID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnDeleteAll)
	defer op.Finish(aErr)

	query := `DELETE FROM pending_messages WHERE session_key = $1`

	_, err := r.pool.Exec(ctx, query, sessionKey(userID, instanceID))
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
