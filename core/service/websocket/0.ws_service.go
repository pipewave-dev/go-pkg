package websocket

import (
	"context"
	"net/http"
	"time"

	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

type ServerDelivery interface {
	/* Mux returns an *http.ServeMux with all WebSocket endpoints pre-registered:
	- POST /issue-tmp-token
		- Input:
			- Header: Authorization
		- Output:
			- Body: string (connection token)
	- GET /gw
		- Input:
			- Query:
				- tk: string (connection token issued by /issue-tmp-token)
		- Output:
			- websocket connection is established
	*/
	Mux() *http.ServeMux
}

// SessionInfo contains information about an active user session.
type SessionInfo struct {
	UserID         string
	InstanceID     string
	HolderID       string
	ConnectionType voWs.WsCoreType
	ConnectedAt    time.Time
	IsAnonymous    bool
}

type WsService interface {
	// SendToSession broadcasts to all containers to find the specific instanceID.
	SendToSession(ctx context.Context, userID string, instanceID string, msgType string, payload []byte) aerror.AError

	// SendToUser broadcasts to all containers to find all sessions of the given userID.
	SendToUser(ctx context.Context, userID string, msgType string, payload []byte) aerror.AError

	// PingConnections actively pings all connected clients to verify liveness.
	// Broadcasts to all containers; removes sessions that do not respond.
	// Browser automatically responds with Pong when:
	//   - Tab is active/focused
	//   - Page is not suspended
	//   - JavaScript engine is running
	//   - WebSocket connection is open
	//   - Browser process is active
	PingConnections()

	Shutdown()

	// DisconnectSession force disconnects a specific session.
	DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError

	// DisconnectUser force disconnects all sessions of a user.
	DisconnectUser(ctx context.Context, userID string) aerror.AError

	// SendToUsers broadcasts to multiple users in a single publish.
	SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError

	CheckOnline(ctx context.Context, userID string) (isOnline bool, aErr aerror.AError)
	// CheckOnlineMultiple checks online status of multiple users at once.
	CheckOnlineMultiple(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError)

	// GetUserSessions returns detailed session info for a user.
	GetUserSessions(ctx context.Context, userID string) ([]SessionInfo, aerror.AError)

	// SendToSessionWithAck sends to a specific session and waits for client acknowledgment.
	SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)

	// SendToUserWithAck sends to a user and waits for client acknowledgment.
	SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)

	SendToAnonymous(ctx context.Context, msgType string, payload []byte, isSendAll bool, instanceID []string) aerror.AError

	SendToAuthenticated(ctx context.Context, msgType string, payload []byte) aerror.AError

	SendToAll(ctx context.Context, msgType string, payload []byte) aerror.AError

	// ResumeSession signals the target container to cancel the ExpiredTimer for a session.
	// Called by the reconnecting container (P2) when a previously temp-disconnected session reconnects.
	ResumeSession(ctx context.Context, targetContainerID, userID, instanceID string) aerror.AError
}
