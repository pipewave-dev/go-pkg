package mediatorsvc

import (
	"context"
	"log/slog"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	fn "github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (m *mediatorSvc) SendToSession(ctx context.Context, userID string, instanceID string, msgType string, payload []byte) aerror.AError {
	pl := br.SendToSessionParams{
		UserId:     userID,
		InstanceId: instanceID,
		MsgType:    msgType,
		Payload:    payload,
	}
	localAction := func() {
		m.broadcastHandler.SendToSession(ctx, pl)
	}
	targetContainerAction := func(containerIDs []string) {
		err := m.broadcast.SendToSession(ctx, containerIDs, pl).Publish()
		if err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast SendToSession",
				slog.String("userID", userID),
				slog.String("instanceID", instanceID),
				slog.Any("containerIDs", containerIDs),
				slog.Any("error", err))
		}
	}
	// When the session is in WsStatusTransferring, wrap the message and save to MessageHub
	// so the client receives it upon reconnect to any container.
	transferringAction := func() aerror.AError {
		id := fn.NewUUID()
		wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(), "", wsSv.MessageType(msgType), payload)
		if err := m.msgHubSvc.Save(ctx, userID, instanceID, wsRes); err != nil {
			return aerror.New(ctx, aerror.ErrUnexpectedDatabase, err)
		}
		return nil
	}

	findThenAction := &findSessionConn{
		ctx:                   ctx,
		userID:                userID,
		instanceID:            instanceID,
		localAction:           localAction,
		targetContainerAction: targetContainerAction,
		transferringAction:    transferringAction,
		callbackNotfound: func() {
			slog.WarnContext(ctx, "InstanceID not found when SendToSession",
				slog.String("userID", userID),
				slog.String("instanceID", instanceID))
		},
		c:              m.c,
		connections:    m.connections,
		activeConnRepo: m.activeConnRepo,
	}

	return findThenAction.findThenAction()
}
