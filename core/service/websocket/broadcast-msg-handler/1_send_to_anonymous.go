package broadcastmsghandler

import (
	"context"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
)

func (h *broadcastMsgHandler) SendToAnonymous(ctx context.Context, payload broadcast.SendToAnonymousParams) {
	id := fn.NewUUID()
	wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(), "",
		wsSv.MessageType(payload.MsgType), payload.Payload)

	if payload.IsSendAll {
		connections := h.connections.GetAllAnonymousConn()
		for _, conn := range connections {
			conn.Send(ctx, wsRes)
		}
		return
	} else {
		instanceIDs := payload.InstanceIds
		if len(instanceIDs) == 0 {
			return
		}

		for _, instanceID := range instanceIDs {
			auth := voAuth.AnonymousUserWebsocketAuth(instanceID)

			conn, ok := h.connections.GetConnection(auth)
			if ok {
				conn.Send(ctx, wsRes)
			}
		}
	}
}
