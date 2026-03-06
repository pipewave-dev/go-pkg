package delivery

import (
	"context"
	"net/http"

	business "github.com/pipewave-dev/go-pkg/core/service/business"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

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

	SendToAnonymous(ctx context.Context, msgType string, payload []byte, isSendAll bool, instanceID []string) aerror.AError

	CheckOnline(ctx context.Context, userID string) (isOnline bool, aErr aerror.AError)

	OnNewRegister() wsSv.OnNewStuffFn
	OnCloseRegister() wsSv.OnCloseStuffFn
}
