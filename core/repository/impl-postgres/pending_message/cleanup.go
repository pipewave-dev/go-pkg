package pendingMessageRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCleanUp = "pendingMessageRepo.CleanUpExpiredPendingMessages"

func (r *pendingMessageRepo) CleanUpExpiredPendingMessages(ctx context.Context) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCleanUp)
	defer op.Finish(aErr)

	query := `DELETE FROM pending_messages WHERE expires_at < NOW()`

	_, err := r.pool.Exec(ctx, query)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
