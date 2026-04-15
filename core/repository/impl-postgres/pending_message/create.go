package pendingMessageRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCreate = "pendingMessageRepo.Create"

func (r *pendingMessageRepo) Create(ctx context.Context, userID, instanceID string, sendAt time.Time, message []byte) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCreate)
	defer op.Finish(aErr)

	expiresAt := time.Now().Add(r.c.Env().ActiveConnection.PendingMsgTTL)

	query := `
		INSERT INTO pending_messages (session_key, send_at, message, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (session_key, send_at) DO NOTHING
	`

	_, err := r.pool.Exec(ctx, query, sessionKey(userID, instanceID), sendAt.UnixNano(), message, expiresAt)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
