package activeConnRepo

import (
	"context"

	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnAddConnection = "activeConnRepo.AddConnection"

func (r *activeConnRepo) AddConnection(ctx context.Context, userID string, sessionID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnAddConnection)
	defer op.Finish(aErr)

	creator := activeConnExp.ActiveConnectionCreator{ConfigStore: r.c}
	_, aErr = creator.Create(ctx, r.ddbC, activeConnExp.CreateParams{
		UserID:    userID,
		SessionID: sessionID,
		HolderID:  r.c.Env().PodName,
	})
	return aErr
}
