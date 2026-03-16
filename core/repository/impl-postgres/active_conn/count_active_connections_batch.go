package activeConnRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCountActiveConnectionsBatch = "activeConnRepo.CountActiveConnectionsBatch"

func (r *activeConnRepo) CountActiveConnectionsBatch(ctx context.Context, userIDs []string) (result map[string]int, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCountActiveConnectionsBatch)
	defer op.Finish(aErr)

	result = make(map[string]int, len(userIDs))
	cutoff := time.Now().Add(-2 * time.Minute)

	query := `
		SELECT user_id, COUNT(*) as cnt FROM active_connections
		WHERE user_id = ANY($1) AND last_heartbeat > $2
		GROUP BY user_id
	`

	rows, err := r.pool.Query(ctx, query, userIDs, cutoff)
	if err != nil {
		return nil, aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID string
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			return nil, aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		}
		result[userID] = count
	}

	// Fill in zeros for users not found
	for _, uid := range userIDs {
		if _, ok := result[uid]; !ok {
			result[uid] = 0
		}
	}

	return result, nil
}
