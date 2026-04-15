package broadcastmsghandler

import (
	"context"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) SendToSessionWithAck(ctx context.Context, payload broadcast.SendToSessionWithAckParams) {
	auth := voAuth.UserWebsocketAuth(payload.UserId, payload.InstanceId)
	conn, ok := h.connections.GetConnection(auth)
	if !ok {
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
	h.sendOrSaveMessageHub(ctx, conn, wsRes.Marshall())
}
