package mediatorsvc

import (
	"context"

	"github.com/pipewave-dev/go-pkg/shared/aerror"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
)

func (m *mediatorSvc) SendToSession(ctx context.Context, userID string, instanceID string, msgType string, payload []byte) aerror.AError {
	pbPayload := br.SendToSessionParams{
		UserId:     userID,
		InstanceId: instanceID,
		MsgType:    msgType,
		Payload:    payload,
	}

	return m.broadcast.SendToSession(ctx, pbPayload).Publish()
}
