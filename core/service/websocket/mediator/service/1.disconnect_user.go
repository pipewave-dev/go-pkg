package mediatorsvc

import (
	"context"
	"log/slog"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) DisconnectUser(ctx context.Context, userID string) aerror.AError {
	pl := br.DisconnectUserParams{
		UserId: userID,
	}

	localAction := func() {
		m.broadcastHandler.DisconnectUser(ctx,
			pl)
	}
	targetContainerAction := func(containerIDs []string) {
		err := m.broadcast.DisconnectUser(ctx, containerIDs, pl).Publish()
		if err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast DisconnectUser",
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
