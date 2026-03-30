package activeConnRepo

import (
	"context"
	"time"

	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/global/constants"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnAddConnection = "activeConnRepo.AddConnection"

func (r *activeConnRepo) AddConnection(ctx context.Context, userID string, sessionID string, connectionType voWs.WsCoreType) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnAddConnection)
	defer op.Finish(aErr)

	now := time.Now()
	ttl := now.Add(2*constants.GlobalHeartbeatRateDuration + time.Second)

	query := `
		INSERT INTO active_connections (user_id, session_id, holder_id, connection_type, status, connected_at, last_heartbeat, ttl)
		VALUES ($1, $2, $3, $4, $5, $6, $6, $7)
		ON CONFLICT (user_id, session_id) DO UPDATE
		SET holder_id = $3, connection_type = $4, status = $5, last_heartbeat = $6, ttl = $7
	`

	_, err := r.pool.Exec(ctx, query, userID, sessionID, r.c.Env().ContainerID, connectionType, voWs.WsStatusConnected, now, ttl)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		return aErr
	}

	return nil
}
