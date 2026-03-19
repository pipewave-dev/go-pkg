package mediatorsvc

import (
	"context"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func (m *mediatorSvc) SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	auth := voAuth.UserWebsocketAuth(userID, instanceID)
	conn, ok := m.connections.GetConnection(auth)
	if !ok {
		return false, nil
	}

	ackID, ch := m.ackManager.CreateAck()

	id := fn.NewUUID()
	wsRes := &wsSv.WebsocketResponse{
		Id:      id.String(),
		MsgType: wsSv.MessageType(msgType),
		Binary:  payload,
		AckId:   ackID,
	}
	conn.Send(wsRes.Marshall())

	return m.ackManager.WaitForAck(ackID, ch, timeout), nil
}

func (m *mediatorSvc) SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	connections := m.connections.GetAllUserConn(userID)
	if len(connections) == 0 {
		return false, nil
	}

	ackID, ch := m.ackManager.CreateAck()

	id := fn.NewUUID()
	wsRes := &wsSv.WebsocketResponse{
		Id:      id.String(),
		MsgType: wsSv.MessageType(msgType),
		Binary:  payload,
		AckId:   ackID,
	}
	data := wsRes.Marshall()

	for _, conn := range connections {
		conn.Send(data)
	}

	return m.ackManager.WaitForAck(ackID, ch, timeout), nil
}
