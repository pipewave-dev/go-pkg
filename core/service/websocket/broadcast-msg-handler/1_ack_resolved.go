package broadcastmsghandler

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
)

func (h *broadcastMsgHandler) AckResolved(ctx context.Context, payload broadcast.AckResolvedParams) {
	h.ackManager.ResolveAck(payload.AckID)
}
