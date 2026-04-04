package activeConnRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGetInstanceConnection = "activeConnRepo.GetInstanceConnection"

func (r *activeConnRepo) GetInstanceConnection(ctx context.Context, userID string, instanceID string) (result *entities.ActiveConnection, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGetInstanceConnection)
	defer op.Finish(aErr)

	querier := activeConnExp.ActiveConnectionQuerier{ConfigStore: r.c}
	return querier.GetByUserAndSession(ctx, r.ddb.Client(), userID, instanceID)
}
