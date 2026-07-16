package pendingMessageRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGetAll = "pendingMessageRepo.GetAll"

func (r *pendingMessageRepo) GetAll(ctx context.Context, userID, instanceID string) (msgs [][]byte, maxSendAt int64, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGetAll)
	defer op.Finish(aErr)

	query := `
		SELECT message, send_at
		FROM pending_messages
		WHERE session_key = $1
		ORDER BY send_at ASC
	`

	rows, err := r.pool.Query(ctx, query, sessionKey(userID, instanceID))
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return nil, 0, aErr
	}
	defer rows.Close()

	for rows.Next() {
		var (
			msg    []byte
			sendAt int64
		)
		if err2 := rows.Scan(&msg, &sendAt); err2 != nil {
			aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err2)
			return nil, 0, aErr
		}
		msgs = append(msgs, msg)
		maxSendAt = sendAt // rows arrive ascending by send_at, so the last one is the max
	}

	if err3 := rows.Err(); err3 != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err3)
		return nil, 0, aErr
	}

	return msgs, maxSendAt, nil
}
