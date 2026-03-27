package broadcastmsghandler

import (
	"context"
	"log/slog"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) SendToUser(ctx context.Context, payload broadcast.SendToUserParams) {
	connections := h.connections.GetAllUserConn(payload.UserId)
	tempSessions := h.msgHubSvc.GetSessions(payload.UserId)

	if len(connections) == 0 && len(tempSessions) == 0 {
		slog.WarnContext(ctx, "SendToUser: no sessions found for user, dropping message",
			slog.String("userID", payload.UserId))
		return
	}

	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(), "",
		wsSv.MessageType(payload.MsgType), payload.Payload)

	for _, conn := range connections {
		conn.Send(wsRes)
	}

	for _, instanceID := range tempSessions {
		slog.WarnContext(ctx, "SendToUser: session temp-disconnected, buffering message",
			slog.String("userID", payload.UserId),
			slog.String("instanceID", instanceID))
		if err := h.msgHubSvc.Save(ctx, payload.UserId, instanceID, wsRes); err != nil {
			slog.ErrorContext(ctx, "SendToUser: failed to buffer message",
				slog.String("userID", payload.UserId),
				slog.String("instanceID", instanceID),
				slog.Any("error", err))
		}
	}
}
