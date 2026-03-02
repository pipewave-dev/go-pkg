package activeConnRepo

import (
	"context"

	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpdateHeartBeat = "activeConnRepo.UpdateHeartBeat"

func (r *activeConnRepo) UpdateHeartBeat(ctx context.Context, userID string, sessionID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpdateHeartBeat)
	defer op.Finish(aErr)

	updater := activeConnExp.ActiveConnectionUpdater{
		ConfigStore: r.c,
	}
	aErr = updater.UpdateLastHeartbeat(ctx, r.ddbC, activeConnExp.UpdateLastHeartbeatParams{
		UserID:    userID,
		SessionID: sessionID,
	})
	return aErr
}
