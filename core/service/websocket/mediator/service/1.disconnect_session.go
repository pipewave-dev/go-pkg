package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError {
	m.activeConnRepo.GetActiveConnections()
	pbPayload := br.DisconnectSessionParams{
		UserId:     userID,
		InstanceId: instanceID,
	}
	return m.broadcast.DisconnectSession(ctx, pbPayload).Publish()
}
