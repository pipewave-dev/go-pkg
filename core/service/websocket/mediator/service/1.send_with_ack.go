package mediatorsvc

import (
	"context"
	"log/slog"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (m *mediatorSvc) SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	ackID, ch := m.ackManager.CreateAck()

	found := false
	localAction := func() {
		auth := voAuth.UserWebsocketAuth(userID, instanceID)
		conn, ok := m.connections.GetConnection(auth)
		if !ok {
			return
		}
		found = true
		id := fn.NewUUID()
		wsRes := wsSv.WebsocketResponse{
			Id:      id.String(),
			MsgType: wsSv.MessageType(msgType),
			Binary:  payload,
			AckId:   ackID,
		}
		conn.Send(wsRes.Marshall())
	}
	targetContainerAction := func(containerIDs []string) {
		found = true
		pl := br.SendToSessionWithAckParams{
			UserId:            userID,
			InstanceId:        instanceID,
			MsgType:           msgType,
			Payload:           payload,
			AckID:             ackID,
			SourceContainerID: m.c.Env().ContainerID,
		}
		if err := m.broadcast.SendToSessionWithAck(ctx, containerIDs, pl).Publish(); err != nil {
			slog.ErrorContext(ctx, "Failed to broadcast SendToSessionWithAck",
				slog.String("userID", userID),
				slog.String("instanceID", instanceID),
				slog.Any("containerIDs", containerIDs),
				slog.Any("error", err))
		}
	}

	findThenAction := &findSessionConn{
		ctx:                   ctx,
		userID:                userID,
		instanceID:            instanceID,
		localAction:           localAction,
		targetContainerAction: targetContainerAction,
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

	found := false
	localAction := func() {
		connections := m.connections.GetAllUserConn(userID)
		if len(connections) == 0 {
			return
		}
		found = true
		id := fn.NewUUID()
		wsRes := wsSv.WebsocketResponse{
			Id:      id.String(),
			MsgType: wsSv.MessageType(msgType),
			Binary:  payload,
			AckId:   ackID,
		}
		data := wsRes.Marshall()
		for _, conn := range connections {
			conn.Send(data)
		}
	}
	targetContainerAction := func(containerIDs []string) {
		found = true
		pl := br.SendToUserWithAckParams{
			UserId:            userID,
			MsgType:           msgType,
			Payload:           payload,
			AckID:             ackID,
			SourceContainerID: m.c.Env().ContainerID,
		}
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
