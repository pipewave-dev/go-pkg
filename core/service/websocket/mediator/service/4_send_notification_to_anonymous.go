package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) SendToAnonymous(ctx context.Context, msgType string, payload []byte, isSendAll bool, instanceIds []string) aerror.AError {
	pbPayload := br.SendToAnonymousParams{
		IsSendAll:   isSendAll,
		InstanceIds: instanceIds,
		MsgType:     msgType,
		Payload:     payload,
	}

	return m.broadcast.SendToAnonymous(ctx, pbPayload).Publish()
}
