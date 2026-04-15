package mediatorsvc

import (
	"context"
	"log/slog"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError {
	pl := br.DisconnectSessionParams{
		UserId:     userID,
		InstanceId: instanceID,
	}
	localAction := func() {
		m.broadcastHandler.DisconnectSession(ctx, pl)
	}
	targetContainerAction := func(containerIDs []string) {
		err := m.broadcast.DisconnectSession(ctx, containerIDs, pl).Publish()
		if err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast DisconnectSession",
				slog.String("userID", userID),
				slog.String("instanceID", instanceID),
				slog.Any("containerIDs", containerIDs),
				slog.Any("error", err))
		}
	}

	findThenAction := &findSessionConn{
		ctx:                   ctx,
		userID:                userID,
		instanceID:            instanceID,
		localAction:           localAction,
		targetContainerAction: targetContainerAction,
		callbackNotfound: func() {
			slog.WarnContext(ctx, "InstanceID not found when DisconnectSession",
				slog.String("userID", userID),
				slog.String("instanceID", instanceID))
			return
		},
		c:              m.c,
		connections:    m.connections,
		activeConnRepo: m.activeConnRepo,
	}

	return findThenAction.findThenAction()
}
