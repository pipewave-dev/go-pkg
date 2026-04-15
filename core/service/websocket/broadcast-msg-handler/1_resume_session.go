package broadcastmsghandler

import (
	"context"
	"log/slog"

	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
)

func (h *broadcastMsgHandler) ResumeSession(ctx context.Context, payload broadcast.ResumeSessionParams) {
	slog.DebugContext(ctx, "ResumeSession: cancelling ExpiredTimer for session",
		slog.String("userID", payload.UserID),
		slog.String("instanceID", payload.InstanceID))
	h.msgHubSvc.Deregister(payload.UserID, payload.InstanceID)
}
