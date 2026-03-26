package mediatorsvc

import (
	"context"
	"log/slog"

	"github.com/pipewave-dev/go-pkg/shared/aerror"

	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
)

func (m *mediatorSvc) SendToSession(ctx context.Context, userID string, instanceID string, msgType string, payload []byte) aerror.AError {
	pl := br.SendToSessionParams{
		UserId:     userID,
		InstanceId: instanceID,
		MsgType:    msgType,
		Payload:    payload,
	}
	localAction := func() {
		m.broadcastHandler.SendToSession(ctx, pl)
	}
	targetContainerAction := func(containerIDs []string) {
		err := m.broadcast.SendToSession(ctx, containerIDs, pl).Publish()
		if err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast SendToSession",
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
			slog.WarnContext(ctx, "InstanceID not found when SendToSession",
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
