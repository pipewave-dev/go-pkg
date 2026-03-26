package mediatorsvc

import (
	"context"
	"log/slog"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError {
	pl := br.SendToUsersParams{
		UserIds: userIDs,
		MsgType: msgType,
		Payload: payload,
	}

	localAction := func() {
		m.broadcastHandler.SendToUsers(ctx,
			pl)
	}
	targetContainerAction := func(containerIDs []string) {
		err := m.broadcast.SendToUsers(ctx, containerIDs, pl).Publish()
		if err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast SendToUsers",
				slog.Any("userIDs", userIDs),
				slog.Any("containerIDs", containerIDs),
				slog.Any("error", err))
		}
	}

	findThenAction := &findMultiUserConn{
		ctx:                   ctx,
		userIDs:               userIDs,
		localAction:           localAction,
		targetContainerAction: targetContainerAction,
		c:                     m.c,
		connections:           m.connections,
		activeConnRepo:        m.activeConnRepo,
	}

	return findThenAction.findThenAction()
}
