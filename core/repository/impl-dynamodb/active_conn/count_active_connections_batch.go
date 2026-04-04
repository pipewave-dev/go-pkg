package activeConnRepo

import (
	"context"

	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnCountActiveConnectionsBatch = "activeConnRepo.CountActiveConnectionsBatch"

func (r *activeConnRepo) CountActiveConnectionsBatch(ctx context.Context, userIDs []string) (result map[string]int, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCountActiveConnectionsBatch)
	defer op.Finish(aErr)

	result = make(map[string]int, len(userIDs))
	querier := activeConnExp.ActiveConnectionQuerier{ConfigStore: r.c}

	for _, userID := range userIDs {
		count, err := querier.CountActive(ctx, r.ddbC, activeConnExp.CountActiveParams{
			UserID:         userID,
			CutOffDuration: r.c.Env().ActiveConnection.HeartbeatCutoff,
		})
		if err != nil {
			return nil, err
		}
		result[userID] = count
	}

	return result, nil
}
