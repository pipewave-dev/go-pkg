package activeConnRepo

import (
	"context"

	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnAddConnection = "activeConnRepo.AddConnection"

func (r *activeConnRepo) AddConnection(ctx context.Context, userID string, instanceID string, connectionType voWs.WsCoreType) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnAddConnection)
	defer op.Finish(aErr)

	creator := activeConnExp.ActiveConnectionCreator{ConfigStore: r.c}
	_, aErr = creator.Create(ctx, r.ddb.Client(), activeConnExp.CreateParams{
		UserID:         userID,
		InstanceID:     instanceID,
		HolderID:       r.c.Env().ContainerID,
		ConnectionType: connectionType,
	})
	return aErr
}
