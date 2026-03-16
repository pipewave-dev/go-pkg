package broadcastmsghandler

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) Broadcast(ctx context.Context, payload broadcast.BroadcastParams) {
	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(),
		"",
		wsSv.MessageType(payload.MsgType),
		payload.Payload)

	var connections []wsSv.WebsocketConn

	switch delivery.BroadcastTarget(payload.Target) {
	case delivery.BroadcastAll:
		connections = h.connections.GetAllConnections()
	case delivery.BroadcastAuthOnly:
		connections = h.connections.GetAllAuthenticatedConn()
	case delivery.BroadcastAnonOnly:
		connections = h.connections.GetAllAnonymousConn()
	default:
		return
	}

	for _, conn := range connections {
		conn.Send(wsRes)
	}
}
