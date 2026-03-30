package delivery

import (
	"context"
	"net/http"
	"time"

	business "github.com/pipewave-dev/go-pkg/core/service/business"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

// BroadcastTarget defines the target audience for a broadcast message.
type BroadcastTarget int

const (
	BroadcastAll      BroadcastTarget = iota // All connections (authenticated + anonymous)
	BroadcastAuthOnly                        // Only authenticated users
	BroadcastAnonOnly                        // Only anonymous connections
)

// SessionInfo is an alias for the websocket package SessionInfo type.
type SessionInfo = wsSv.SessionInfo

// ModuleDelivery is the main interface exposed by pipewave. External Go services embed it as a module.
type ModuleDelivery interface {
	SetFns(fns *configprovider.Fns)

	Mux() *http.ServeMux
	Services() ExportedServices
	Monitoring() business.Monitoring
	IsHealthy() bool
	Shutdown()
}

type ExportedServices interface {
	// SendToSession broadcasts to all containers to find the specific instanceID.
	SendToSession(ctx context.Context, userID string, instanceID string, msgType string, payload []byte) aerror.AError

	// SendToUser broadcasts to all containers to find all sessions of the given userID.
	SendToUser(ctx context.Context, userID string, msgType string, payload []byte) aerror.AError

	// PingConnections actively pings all connected clients to verify liveness.
	// Broadcasts to all containers; removes sessions that do not respond.
	// Not work when connection type is long-polling
	// Browser automatically responds with Pong when:
	//   - Tab is active/focused
	//   - Page is not suspended
	//   - JavaScript engine is running
	//   - WebSocket connection is open
	//   - Browser process is active
	PingConnections()

	CheckOnline(ctx context.Context, userID string) (isOnline bool, aErr aerror.AError)

	OnNewRegister() wsSv.OnNewStuffFn
	OnCloseRegister() wsSv.OnCloseStuffFn

	// DisconnectSession force disconnects a specific session.
	DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError

	// DisconnectUser force disconnects all sessions of a user.
	DisconnectUser(ctx context.Context, userID string) aerror.AError

	// SendToUsers broadcasts to multiple users in a single publish.
	SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError

	// CheckOnlineMultiple checks online status of multiple users at once.
	CheckOnlineMultiple(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError)

	// GetUserSessions returns detailed session info for a user.
	GetUserSessions(ctx context.Context, userID string) ([]SessionInfo, aerror.AError)

	// SendToSessionWithAck sends to a specific session and waits for client acknowledgment.
	SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)

	// SendToUserWithAck sends to a user and waits for client acknowledgment. (experimental, may be removed in the future)
	SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError)

	SendToAll(ctx context.Context, msgType string, payload []byte) aerror.AError

	SendToAnonymous(ctx context.Context, msgType string, payload []byte, isSendAll bool, instanceID []string) aerror.AError

	SendToAuthenticated(ctx context.Context, msgType string, payload []byte) aerror.AError
}
