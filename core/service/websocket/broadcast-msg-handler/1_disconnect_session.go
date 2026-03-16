package broadcastmsghandler

import (
	"context"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
)

func (h *broadcastMsgHandler) DisconnectSession(ctx context.Context, payload broadcast.DisconnectSessionParams) {
	auth := voAuth.UserWebsocketAuth(payload.UserId, payload.InstanceId)

	conn, ok := h.connections.GetConnection(auth)
	if !ok {
		return
	}

	conn.Close()
}
