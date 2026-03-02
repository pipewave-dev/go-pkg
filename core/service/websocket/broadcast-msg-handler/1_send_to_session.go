package broadcastmsghandler

import (
	"context"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) SendToSession(ctx context.Context, payload broadcast.SendToSessionParams) {
	auth := voAuth.UserWebsocketAuth(
		payload.UserId,
		payload.InstanceId,
	)

	conn, ok := h.connections.GetConnection(auth)
	if !ok {
		return
	}

	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(), "",
		wsSv.MessageType(payload.MsgType), payload.Payload)
	conn.Send(wsRes)
}
