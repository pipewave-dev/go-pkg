package pendingMessageRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnDeleteUpTo = "pendingMessageRepo.DeleteUpTo"

// DeleteUpTo deletes only messages with send_at <= maxSendAt, so it can safely follow
// GetAll without deleting a message Created concurrently after that read.
func (r *pendingMessageRepo) DeleteUpTo(ctx context.Context, userID, instanceID string, maxSendAt int64) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnDeleteUpTo)
	defer op.Finish(aErr)

	query := `DELETE FROM pending_messages WHERE session_key = $1 AND send_at <= $2`

	_, err := r.pool.Exec(ctx, query, sessionKey(userID, instanceID), maxSendAt)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
