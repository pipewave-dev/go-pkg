package activeConnRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGetActiveConnectionsByUserIDs = "activeConnRepo.GetActiveConnectionsByUserIDs"

func (r *activeConnRepo) GetActiveConnectionsByUserIDs(ctx context.Context, userIDs []string) (result []entities.ActiveConnection, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGetActiveConnectionsByUserIDs)
	defer op.Finish(aErr)

	querier := activeConnExp.ActiveConnectionQuerier{ConfigStore: r.c}

	for _, userID := range userIDs {
		items, err := querier.QueryByUserID(ctx, r.ddb.Client(), userID)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}

	return result, nil
}
