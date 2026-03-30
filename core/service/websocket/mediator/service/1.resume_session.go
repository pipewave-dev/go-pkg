package mediatorsvc

import (
	"context"
	"log/slog"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) ResumeSession(ctx context.Context, targetContainerID, userID, instanceID string) aerror.AError {
	pl := br.ResumeSessionParams{
		UserID:     userID,
		InstanceID: instanceID,
	}
	localAction := func() {
		m.broadcastHandler.ResumeSession(ctx, pl)
	}
	targetContainerAction := func(containerIDs []string) {
		err := m.broadcast.ResumeSession(ctx, containerIDs, pl).Publish()
		if err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast ResumeSession",
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
			slog.WarnContext(ctx, "InstanceID not found when ResumeSession",
				slog.String("userID", userID),
				slog.String("instanceID", instanceID))
		},
		c:              m.c,
		connections:    m.connections,
		activeConnRepo: m.activeConnRepo,
	}

	return findThenAction.findThenAction()
}
