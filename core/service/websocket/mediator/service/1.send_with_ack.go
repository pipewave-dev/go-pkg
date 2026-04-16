package mediatorsvc

import (
	"context"
	"log/slog"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	fn "github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (m *mediatorSvc) SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	ackID, ch := m.ackManager.CreateAck()
	pl := br.SendToSessionWithAckParams{
		UserId:            userID,
		InstanceId:        instanceID,
		MsgType:           msgType,
		Payload:           payload,
		AckID:             ackID,
		SourceContainerID: m.c.Env().Info.ContainerID,
	}

	found := false
	localAction := func() {
		auth := voAuth.UserWebsocketAuth(userID, instanceID)
		_, ok := m.connections.GetConnection(auth)
		if !ok {
			return
		}
		found = true
		m.broadcastHandler.SendToSessionWithAck(ctx, pl)
	}
	targetContainerAction := func(containerIDs []string) {
		if len(containerIDs) == 0 {
			return
		}
		found = true
		if err := m.broadcast.SendToSessionWithAck(ctx, containerIDs, pl).Publish(); err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast SendToSessionWithAck",
				slog.String("userID", userID),
				slog.String("instanceID", instanceID),
				slog.Any("containerIDs", containerIDs),
				slog.Any("error", err))
		}
	}

	// When the session is in WsStatusTransferring, buffer message to MessageHub.
	// ACK will not arrive (session is reconnecting); caller receives found=false.
	transferringAction := func() aerror.AError {
		id := fn.NewUUID()
		wsRes := wsSv.WrapperBytesToWebsocketResponse(id.String(), "", wsSv.MessageType(pl.MsgType), pl.Payload)
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
		callbackNotfound:      func() {},
		c:                     m.c,
		connections:           m.connections,
		activeConnRepo:        m.activeConnRepo,
	}

	if aErr = findThenAction.findThenAction(); aErr != nil {
		m.ackManager.ResolveAck(ackID)
		return false, aErr
	}
	if !found {
		m.ackManager.ResolveAck(ackID)
		return false, nil
	}

	return m.ackManager.WaitForAck(ackID, ch, timeout), nil
}

func (m *mediatorSvc) SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	ackID, ch := m.ackManager.CreateAck()
	pl := br.SendToUserWithAckParams{
		UserId:            userID,
		MsgType:           msgType,
		Payload:           payload,
		AckID:             ackID,
		SourceContainerID: m.c.Env().Info.ContainerID,
	}

	found := false
	localAction := func() {
		conns := m.connections.GetAllUserConn(userID)
		if len(conns) == 0 {
			return
		}
		found = true
		m.broadcastHandler.SendToUserWithAck(ctx, pl)
	}
	targetContainerAction := func(containerIDs []string) {
		if len(containerIDs) == 0 {
			return
		}
		found = true
		if err := m.broadcast.SendToUserWithAck(ctx, containerIDs, pl).Publish(); err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast SendToUserWithAck",
				slog.String("userID", userID),
				slog.Any("containerIDs", containerIDs),
				slog.Any("error", err))
		}
	}

	findThenAction := &findUserConn{
		ctx:                   ctx,
		userID:                userID,
		localAction:           localAction,
		targetContainerAction: targetContainerAction,
		c:                     m.c,
		connections:           m.connections,
		activeConnRepo:        m.activeConnRepo,
	}

	if aErr = findThenAction.findThenAction(); aErr != nil {
		m.ackManager.ResolveAck(ackID)
		return false, aErr
	}
	if !found {
		m.ackManager.ResolveAck(ackID)
		return false, nil
	}

	return m.ackManager.WaitForAck(ackID, ch, timeout), nil
}
