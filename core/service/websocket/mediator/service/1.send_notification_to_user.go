package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

// SendToUser broadcasts across all containers to deliver to the given userID.
func (m *mediatorSvc) SendToUser(ctx context.Context, userID string, msgType string, payload []byte) aerror.AError {
	pbPayload := br.SendToUserParams{
		UserId:  userID,
		MsgType: msgType,
		Payload: payload,
	}

	return m.broadcast.SendToUser(ctx, pbPayload).Publish()
}
