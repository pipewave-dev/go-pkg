package broadcastmsghandler

import (
	"context"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (h *broadcastMsgHandler) SendToUsers(ctx context.Context, payload broadcast.SendToUsersParams) {
	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(),
		"",
		wsSv.MessageType(payload.MsgType),
		payload.Payload)

	for _, userID := range payload.UserIds {
		connections := h.connections.GetAllUserConn(userID)
		for _, conn := range connections {
			conn.Send(wsRes)
		}
	}
}
