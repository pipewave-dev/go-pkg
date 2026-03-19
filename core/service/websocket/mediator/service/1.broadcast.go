package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) Broadcast(ctx context.Context, target int, msgType string, payload []byte) aerror.AError {
	pbPayload := br.BroadcastParams{
		Target:  target,
		MsgType: msgType,
		Payload: payload,
	}
	return m.broadcast.Broadcast(ctx, pbPayload).Publish()
}
