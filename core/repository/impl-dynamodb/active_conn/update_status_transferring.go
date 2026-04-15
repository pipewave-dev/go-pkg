package activeConnRepo

import (
	"context"

	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpdateStatusTransferring = "activeConnRepo.UpdateStatusTransferring"

func (r *activeConnRepo) UpdateStatusTransferring(ctx context.Context, userID string, instanceID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpdateStatusTransferring)
	defer op.Finish(aErr)

	updater := activeConnExp.ActiveConnectionUpdater{ConfigStore: r.c}
	aErr = updater.UpdateStatusTransferring(ctx, r.ddb.Client(), userID, instanceID)
	return aErr
}
