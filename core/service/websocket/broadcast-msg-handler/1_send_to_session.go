package broadcastmsghandler

import (
	"context"
	"log/slog"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) SendToSession(ctx context.Context, payload broadcast.SendToSessionParams) {
	auth := voAuth.UserWebsocketAuth(payload.UserId, payload.InstanceId)

	// Build the wrapped WS frame once — used for both live delivery and buffering.
	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(), "",
		wsSv.MessageType(payload.MsgType), payload.Payload)

	conn, ok := h.connections.GetConnection(auth)
	if !ok {
		if h.msgHubSvc.IsRegistered(payload.UserId, payload.InstanceId) {
			// Session is temp-disconnected on this container — buffer the pre-wrapped frame.
			if err := h.msgHubSvc.Save(ctx, payload.UserId, payload.InstanceId, wsRes); err != nil {
				slog.WarnContext(ctx, "SendToSession: failed to buffer message for temp-disconnected session",
					slog.String("userID", payload.UserId),
					slog.String("instanceID", payload.InstanceId),
					slog.Any("error", err))
			}
		} else {
			slog.WarnContext(ctx, "SendToSession: session not found, dropping message",
				slog.String("userID", payload.UserId),
				slog.String("instanceID", payload.InstanceId))
		}
		return
	}

	h.sendOrSaveMessageHub(ctx, conn, wsRes)
}
