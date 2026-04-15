package mediatorsvc

import (
	"context"
	"log/slog"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

// SendToUser broadcasts across all containers to deliver to the given userID.
func (m *mediatorSvc) SendToUser(ctx context.Context, userID string, msgType string, payload []byte) aerror.AError {
	pl := br.SendToUserParams{
		UserId:  userID,
		MsgType: msgType,
		Payload: payload,
	}

	localAction := func() {
		m.broadcastHandler.SendToUser(ctx,
			pl)
	}
	targetContainerAction := func(containerIDs []string) {
		err := m.broadcast.SendToUser(ctx, containerIDs, pl).Publish()
		if err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast SendToUser",
				slog.String("userID", userID),
				slog.Any("containerIDs", containerIDs),
				slog.Any("error", err))
		}
	}

	findThenAction := &findUserConn{
		ctx:                   ctx,
		userID:                userID,
		localAction:           localAction,
		targetContainerAction: targetContainerAction,
		c:                     m.c,
		connections:           m.connections,
		activeConnRepo:        m.activeConnRepo,
	}

	return findThenAction.findThenAction()
}
