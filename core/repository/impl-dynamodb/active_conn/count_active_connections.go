package activeConnRepo

import (
	"context"

	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCountActiveConnections = "activeConnRepo.CountActiveConnections"

func (r *activeConnRepo) CountActiveConnections(ctx context.Context, userID string) (count int, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCountActiveConnections)
	defer op.Finish(aErr)

	querier := activeConnExp.ActiveConnectionQuerier{ConfigStore: r.c}
	count, aErr = querier.CountActive(ctx, r.ddbC, activeConnExp.CountActiveParams{
		UserID:         userID,
		CutOffDuration: r.c.Env().HeartbeatCutoff,
	})
	return count, aErr
}
