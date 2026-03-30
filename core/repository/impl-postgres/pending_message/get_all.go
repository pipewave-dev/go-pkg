package pendingMessageRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGetAll = "pendingMessageRepo.GetAll"

func (r *pendingMessageRepo) GetAll(ctx context.Context, userID, instanceID string) (msgs [][]byte, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGetAll)
	defer op.Finish(aErr)

	query := `
		SELECT message
		FROM pending_messages
		WHERE session_key = $1
		ORDER BY send_at ASC
	`

	rows, err := r.pool.Query(ctx, query, sessionKey(userID, instanceID))
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return nil, aErr
	}
	defer rows.Close()

	for rows.Next() {
		var msg []byte
		if err2 := rows.Scan(&msg); err2 != nil {
			aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err2)
			return nil, aErr
		}
		msgs = append(msgs, msg)
	}

	if err3 := rows.Err(); err3 != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err3)
		return nil, aErr
	}

	return msgs, nil
}
