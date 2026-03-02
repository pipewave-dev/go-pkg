package userRepo

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/user/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpdateLastHeartbeat = "userRepo.UpdateLastHeartbeat"

func (r *userRepo) UpdateLastHeartbeat(ctx context.Context, userID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpdateLastHeartbeat)
	defer op.Finish(aErr)

	updater := exprbuilder.UserUpdater{
		ConfigStore: r.cfg,
	}
	aErr = updater.UpdateLastHeartbeat(ctx, r.ddbC, exprbuilder.UpdateLastHeartbeatParams{
		ID:              userID,
		LastHeartbeatAt: time.Now(),
	})
	return aErr
}
