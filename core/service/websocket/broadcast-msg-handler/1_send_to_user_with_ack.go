package broadcastmsghandler

import (
	"context"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) SendToUserWithAck(ctx context.Context, payload broadcast.SendToUserWithAckParams) {
	connections := h.connections.GetAllUserConn(payload.UserId)
	if len(connections) == 0 {
		return
	}

	h.ackManager.RegisterRemoteAck(payload.AckID, payload.SourceContainerID)

	id := fn.NewUUID()
	wsRes := wsSv.WebsocketResponse{
		Id:      id.String(),
		MsgType: wsSv.MessageType(payload.MsgType),
		Binary:  payload.Payload,
		AckId:   payload.AckID,
	}
	data := wsRes.Marshall()
	for _, conn := range connections {
		conn.Send(data)
	}
}
