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

	if targetContainerID == "" {
		slog.WarnContext(ctx, "ResumeSession called without target container",
			slog.String("userID", userID),
			slog.String("instanceID", instanceID))
		return nil
	}

	if targetContainerID == m.c.Env().Info.ContainerID {
		m.broadcastHandler.ResumeSession(ctx, pl)
		return nil
	}

	err := m.broadcast.ResumeSession(ctx, []string{targetContainerID}, pl).Publish()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to broadcast ResumeSession",
			slog.String("userID", userID),
			slog.String("instanceID", instanceID),
			slog.String("targetContainerID", targetContainerID),
			slog.Any("error", err))
	}

	return nil
}
