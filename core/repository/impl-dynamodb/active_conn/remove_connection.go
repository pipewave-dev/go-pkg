package activeConnRepo

import (
	"context"

	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnRemoveConnection = "activeConnRepo.RemoveConnection"

func (r *activeConnRepo) RemoveConnection(ctx context.Context, userID string, instanceID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnRemoveConnection)
	defer op.Finish(aErr)

	cleaner := activeConnExp.ActiveConnectionDeleter{ConfigStore: r.c}
	aErr = cleaner.Delete(ctx, r.ddbC, activeConnExp.DeleteParams{
		UserID:     userID,
		InstanceID: instanceID,
	})
	return aErr
}
