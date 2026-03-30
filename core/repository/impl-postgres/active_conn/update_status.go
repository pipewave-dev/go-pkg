package activeConnRepo

import (
	"context"

	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (r *activeConnRepo) UpdateStatus(ctx context.Context, userID string, instanceID string, status voWs.WsStatus) aerror.AError {
	// TODO
	panic("ActiveConnStore.UpdateStatus not implemented — add Postgres implementation")
}
