package activeConnRepo

import (
	"context"

	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpdateStatus = "activeConnRepo.UpdateStatus"

func (r *activeConnRepo) UpdateStatus(ctx context.Context, userID string, instanceID string, status voWs.WsStatus) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpdateStatus)
	defer op.Finish(aErr)

	updater := activeConnExp.ActiveConnectionUpdater{ConfigStore: r.c}
	aErr = updater.UpdateStatus(ctx, r.ddbC, activeConnExp.UpdateStatusParams{
		UserID:     userID,
		InstanceID: instanceID,
		Status:     status,
	})
	return aErr
}
