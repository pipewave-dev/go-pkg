package mediatorsvc

import (
	"context"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) DisconnectUser(ctx context.Context, userID string) aerror.AError {
	pbPayload := br.DisconnectUserParams{
		UserId: userID,
	}
	return m.broadcast.DisconnectUser(ctx, pbPayload).Publish()
}
