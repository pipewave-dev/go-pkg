package userRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/user/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnUpsert = "userRepo.Upsert"

func (r *userRepo) Upsert(ctx context.Context, userID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnUpsert)
	defer op.Finish(aErr)

	creator := exprbuilder.UserCreator{
		ConfigStore: r.cfg,
	}
	_, aErr = creator.Upsert(ctx, r.ddbC, exprbuilder.UpsertParams{
		ID: userID,
	})
	return aErr
}
