package activeConnRepo

import (
	"context"

	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCountTotalActiveConnections = "activeConnRepo.CountTotalActiveConnections"

func (r *activeConnRepo) CountTotalActiveConnections(ctx context.Context) (total int64, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCountTotalActiveConnections)
	defer op.Finish(aErr)

	querier := activeConnExp.ActiveConnectionQuerier{ConfigStore: r.c}
	total, aErr = querier.CountTotalActive(ctx, r.ddbC, activeConnExp.CountTotalActiveParams{
		CutOffDuration: r.c.Env().HeartbeatCutoff,
	})
	return total, aErr
}
