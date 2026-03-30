package gobwas

import (
	"context"
	"fmt"
	"net"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	healthyprovider "github.com/pipewave-dev/go-pkg/provider/healthy-provider"

	"github.com/mailru/easygo/netpoll"
)

type NetpollServer struct {
	c configprovider.ConfigStore

	poller      netpoll.Poller
	healthy     healthyprovider.Healthy
	connections int64
	stats       *serverStats
	workerPool  *workerpool.WorkerPool

	onTextMessage wsSv.OnTextMessageFn
	onBinMessage  wsSv.OnBinMessageFn
	onReadError   wsSv.OnReadErrorFn
	onWriteError  wsSv.OnWriteErrorFn
	onClose       wsSv.OnCloseStuffFn
}

// GobwasConnection represents a single WebSocket client connection.
type GobwasConnection struct {
	c      configprovider.ConfigStore
	conn   net.Conn
	server *NetpollServer
	auth   voAuth.WebsocketAuth
	desc   *netpoll.Desc
	closed int32
}

func (cl *GobwasConnection) CoreType() voWs.WsCoreType {
	return voWs.WsCoreGobwas
}

func (cl *GobwasConnection) Ping() {
	if cl.server != nil {
		cl.server.ping(cl)
	}
}

func (cl *GobwasConnection) Auth() voAuth.WebsocketAuth {
	return cl.auth
}

func (cl *GobwasConnection) Send(payload []byte) error {
	if cl.server != nil {
		return cl.server.send(cl, payload)
	}
	return fmt.Errorf("connection is not associated with a server")
}

func (cl *GobwasConnection) Close() {
	if cl.server != nil {
		cl.server.removeClient(cl)
	}
	if cl.c.Env().Fns.OnCloseConnection != nil {
		cl.c.Env().Fns.OnCloseConnection.OnCloseConnection(context.Background(), cl.auth)
	}
}

// serverStats tracks server performance metrics.
type serverStats struct {
	ConnectionsAccepted int64
	ConnectionsClosed   int64
	StartTime           time.Time
}
