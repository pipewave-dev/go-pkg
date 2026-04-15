package broadcastmsghandler

import (
	"context"
	"log/slog"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
)

func (h *broadcastMsgHandler) sendOrSaveMessageHub(ctx context.Context, conn wsSv.WebsocketConn, payload []byte) {
	err := conn.Send(ctx, payload)
	if err != nil {
		ctx = context.WithoutCancel(ctx)
		auth := conn.Auth()
		h.wp.Submit(func() {
			h.saveMessageHub(ctx, auth, payload, 0)
		})
	}
}

const maxRetryNumber = 5

func (h *broadcastMsgHandler) saveMessageHub(ctx context.Context, auth voAuth.WebsocketAuth, payload []byte, retryNumber int) {
	if retryNumber >= maxRetryNumber {
		slog.ErrorContext(ctx, "max retry number reached. Dropping message",
			slog.String("userID", auth.UserID),
			slog.String("instanceID", auth.InstanceID))
		return
	}
	activeCon, err := h.storeActiveWs.GetInstanceConnection(ctx, auth.UserID, auth.InstanceID)
	if err != nil {
		slog.ErrorContext(ctx, "storeActiveWs error",
			slog.Any("error", err))
		return
	}
	if activeCon == nil {
		slog.WarnContext(ctx, "not found connection in storeActiveWs. Dropping message",
			slog.String("userID", auth.UserID),
			slog.String("instanceID", auth.InstanceID))
		return
	}
	if activeCon.Status == voWs.WsStatusConnected {
		slog.WarnContext(ctx, "connection is still active but failed to send message. Maybe database is slow. Saving message to hub with delay",
			slog.String("userID", auth.UserID),
			slog.String("instanceID", auth.InstanceID),
			slog.Int("retryNumber", retryNumber))
		delay := time.Duration((retryNumber+1)*750) * time.Millisecond
		time.AfterFunc(
			min(delay, 3*time.Second), // max delay 3 seconds
			func() {
				h.wp.Submit(func() {
					h.saveMessageHub(ctx, auth, payload, retryNumber+1)
				})
			})
	} else {
		err := h.msgHubSvc.Save(ctx,
			auth.UserID,
			auth.InstanceID,
			payload)
		if err != nil {
			slog.ErrorContext(ctx, "msgHubSvc.Save error",
				slog.Any("error", err))
			return
		}
	}
}
