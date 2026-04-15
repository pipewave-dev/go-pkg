package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) SendToAll(ctx context.Context, msgType string, payload []byte) aerror.AError {
	return m.broadcast.SendToAll(ctx, br.SendToAllParams{
		MsgType: msgType,
		Payload: payload,
	}).Publish()
}
