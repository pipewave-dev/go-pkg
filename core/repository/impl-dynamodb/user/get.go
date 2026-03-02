package userRepo

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	"github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/user/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnGet = "userRepo.Get"

func (r *userRepo) Get(ctx context.Context, userID string) (user entities.User, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnGet)
	defer op.Finish(aErr)

	querier := exprbuilder.UserQuerier{
		ConfigStore: r.cfg,
	}
	result, aErr := querier.ByID(ctx, r.ddbC, exprbuilder.ByIDParams{
		ID: userID,
	})
	if aErr != nil {
		return entities.User{}, aErr
	}

	return *result, nil
}
