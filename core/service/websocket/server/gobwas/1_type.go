package gobwas

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
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
	c       configprovider.ConfigStore
	conn    net.Conn
	server  *NetpollServer
	auth    voAuth.WebsocketAuth
	desc    *netpoll.Desc
	closed  int32
	closeTx int32
	closeRx int32
	writeMu sync.Mutex
	stateMu sync.Mutex
	// lastReadAt tracks the last successfully received frame of any kind.
	lastReadAt time.Time
	lastPingAt time.Time
	lastPongAt time.Time
	// awaitingPong is set after a server ping is sent and cleared on pong.
	awaitingPong bool
	drainMu      sync.RWMutex
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

func (cl *GobwasConnection) Send(ctx context.Context, payload []byte) error {
	cl.drainMu.RLock()
	defer cl.drainMu.RUnlock()
	if cl.server != nil {
		return cl.server.send(ctx, cl, payload)
	}
	return fmt.Errorf("connection is not associated with a server")
}

// BeginDrain acquires an exclusive lock, blocking all concurrent Send() calls.
func (cl *GobwasConnection) BeginDrain() { cl.drainMu.Lock() }

// EndDrain releases the exclusive lock, allowing blocked Send() calls to proceed.
func (cl *GobwasConnection) EndDrain() { cl.drainMu.Unlock() }

// SendDirect writes directly to the server without acquiring drainMu.
// Must only be called between BeginDrain/EndDrain.
func (cl *GobwasConnection) SendDirect(ctx context.Context, payload []byte) error {
	if cl.server != nil {
		return cl.server.send(ctx, cl, payload)
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

func (cl *GobwasConnection) MarkCloseSentIfFirst() bool {
	return atomic.CompareAndSwapInt32(&cl.closeTx, 0, 1)
}

func (cl *GobwasConnection) MarkCloseReceived() {
	atomic.StoreInt32(&cl.closeRx, 1)
}

func (cl *GobwasConnection) noteRead(now time.Time) {
	cl.stateMu.Lock()
	defer cl.stateMu.Unlock()
	cl.lastReadAt = now
	cl.awaitingPong = false
}

func (cl *GobwasConnection) notePong(now time.Time) {
	cl.stateMu.Lock()
	defer cl.stateMu.Unlock()
	cl.lastReadAt = now
	cl.lastPongAt = now
	cl.awaitingPong = false
}

type pingAction int8

const (
	pingActionSkip pingAction = iota
	pingActionSend
	pingActionClose
)

func (cl *GobwasConnection) nextPingAction() pingAction {
	pingIdleAfter := cl.c.Env().PingChecker.PingIdleAfter
	pongTimeout := cl.c.Env().PingChecker.PongTimeout
	now := time.Now()

	cl.stateMu.Lock()
	defer cl.stateMu.Unlock()

	if cl.awaitingPong {
		if now.Sub(cl.lastPingAt) >= pongTimeout {
			return pingActionClose
		}
		return pingActionSkip
	}

	if now.Sub(cl.lastReadAt) < pingIdleAfter {
		return pingActionSkip
	}

	cl.awaitingPong = true
	cl.lastPingAt = now
	return pingActionSend
}

// serverStats tracks server performance metrics.
type serverStats struct {
	ConnectionsAccepted int64
	ConnectionsClosed   int64
	StartTime           time.Time
}
