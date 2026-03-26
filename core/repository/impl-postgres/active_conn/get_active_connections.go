package activeConnRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGetActiveConnections = "activeConnRepo.GetActiveConnections"

func (r *activeConnRepo) GetActiveConnections(ctx context.Context, userID string) (result []entities.ActiveConnection, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGetActiveConnections)
	defer op.Finish(aErr)

	cutoff := time.Now().Add(-r.c.Env().HeartbeatCutoff)

	query := `
		SELECT user_id, session_id, holder_id, connection_type, status, connected_at, last_heartbeat, ttl
		FROM active_connections
		WHERE user_id = $1 AND last_heartbeat > $2
	`

	rows, err := r.pool.Query(ctx, query, userID, cutoff)
	if err != nil {
		return nil, aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
	}
	defer rows.Close()

	for rows.Next() {
		var ac entities.ActiveConnection
		if err := rows.Scan(&ac.UserID, &ac.SessionID, &ac.HolderID, &ac.ConnectionType, &ac.Status, &ac.ConnectedAt, &ac.LastHeartbeat, &ac.TTL); err != nil {
			return nil, aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		}
		result = append(result, ac)
	}

	return result, nil
}
