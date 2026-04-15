package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) SendToAuthenticated(ctx context.Context, msgType string, payload []byte) aerror.AError {
	return m.broadcast.SendToAuthenticated(ctx, br.SendToAuthenticatedParams{
		MsgType: msgType,
		Payload: payload,
	}).Publish()
}
