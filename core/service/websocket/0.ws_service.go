package websocket

import (
	"context"
	"net/http"

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

	SendToAnonymous(ctx context.Context, msgType string, payload []byte, isSendAll bool, instanceID []string) aerror.AError

	Shutdown()
}
