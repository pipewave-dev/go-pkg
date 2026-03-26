package mediatorsvc

import (
	"context"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) GetUserSessions(ctx context.Context, userID string) ([]wsSv.SessionInfo, aerror.AError) {
	connections, aErr := m.activeConnRepo.GetActiveConnections(ctx, userID)
	if aErr != nil {
		return nil, aErr
	}

	sessions := make([]wsSv.SessionInfo, 0, len(connections))
	for _, conn := range connections {
		sessions = append(sessions, wsSv.SessionInfo{
			UserID:         userID,
			InstanceID:     conn.SessionID,
			HolderID:       conn.HolderID,
			ConnectionType: conn.ConnectionType,
			ConnectedAt:    conn.ConnectedAt,
			IsAnonymous:    conn.UserID == "",
		})
	}
	return sessions, nil
}
