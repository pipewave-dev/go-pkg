package broadcastmsghandler

import (
	"context"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) SendToAuthenticated(ctx context.Context, payload broadcast.SendToAuthenticatedParams) {
	connections := h.connections.GetAllAuthenticatedConn()
	if len(connections) == 0 {
		return
	}

	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(),
		"",
		wsSv.MessageType(payload.MsgType),
		payload.Payload)

	for _, conn := range connections {
		h.sendOrSaveMessageHub(ctx, conn, wsRes)
	}
}
