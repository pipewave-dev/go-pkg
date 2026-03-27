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
	aErr := m.broadcast.ResumeSession(ctx, []string{targetContainerID}, pl).Publish()
	if aErr != nil {
		slog.ErrorContext(ctx, "ResumeSession: failed to publish",
			slog.String("targetContainerID", targetContainerID),
			slog.String("userID", userID),
			slog.String("instanceID", instanceID),
			slog.Any("error", aErr))
		return aErr
	}
	return nil
}
