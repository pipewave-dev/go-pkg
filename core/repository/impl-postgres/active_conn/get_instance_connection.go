package activeConnRepo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGetInstanceConnection = "activeConnRepo.GetInstanceConnection"

func (r *activeConnRepo) GetInstanceConnection(ctx context.Context, userID string, instanceID string) (result *entities.ActiveConnection, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGetInstanceConnection)
	defer op.Finish(aErr)

	cutoff := time.Now().Add(-r.c.Env().HeartbeatCutoff)
	query := `
		SELECT user_id, session_id, holder_id, connected_at, last_heartbeat, ttl
		FROM active_connections
		WHERE user_id = $1 AND session_id = $2 AND last_heartbeat > $3
	`

	var ac entities.ActiveConnection
	err := r.pool.QueryRow(ctx, query, userID, instanceID, cutoff).Scan(
		&ac.UserID, &ac.SessionID, &ac.HolderID, &ac.ConnectedAt, &ac.LastHeartbeat, &ac.TTL,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, aerror.New(ctx, aerror.RecordNotFound, nil)
		}
		return nil, aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
	}

	return &ac, nil
}
