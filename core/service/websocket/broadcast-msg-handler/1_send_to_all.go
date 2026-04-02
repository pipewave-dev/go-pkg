package broadcastmsghandler

import (
	"context"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) SendToAll(ctx context.Context, payload broadcast.SendToAllParams) {
	connections := h.connections.GetAllConnections()
	if len(connections) == 0 {
		return
	}

	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(),
		"",
		wsSv.MessageType(payload.MsgType),
		payload.Payload)

	for _, conn := range connections {
		conn.Send(ctx, wsRes)
	}
}
