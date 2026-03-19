package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError {
	pbPayload := br.SendToUsersParams{
		UserIds: userIDs,
		MsgType: msgType,
		Payload: payload,
	}
	return m.broadcast.SendToUsers(ctx, pbPayload).Publish()
}
