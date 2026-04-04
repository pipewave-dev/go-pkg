package activeConnRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGetActiveConnections = "activeConnRepo.GetActiveConnections"

func (r *activeConnRepo) GetActiveConnections(ctx context.Context, userID string) (result []entities.ActiveConnection, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGetActiveConnections)
	defer op.Finish(aErr)

	querier := activeConnExp.ActiveConnectionQuerier{ConfigStore: r.c}
	items, err := querier.QueryByUserID(ctx, r.ddb.Client(), userID)
	if err != nil {
		return nil, err
	}

	return items, nil
}
