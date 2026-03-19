package broadcastmsghandler

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
)

func (h *broadcastMsgHandler) DisconnectUser(ctx context.Context, payload broadcast.DisconnectUserParams) {
	connections := h.connections.GetAllUserConn(payload.UserId)
	for _, conn := range connections {
		conn.Close()
	}
}
