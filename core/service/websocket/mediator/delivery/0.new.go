package delivery

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/server/gobwas"
	"github.com/pipewave-dev/go-pkg/pkg/queue"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	healthyprovider "github.com/pipewave-dev/go-pkg/provider/healthy-provider"
)

type serverDelivery struct {
	c configprovider.ConfigStore

	// HTTP multiplexer
	mux *http.ServeMux

	// Worker pool (singleton)
	workerPool *workerpool.WorkerPool

	// Services
	wsService        wsSv.WsService
	connectionMgr    wsSv.ConnectionManager
	rateLimiter      wsSv.RateLimiter
	clientMsgHandler wsSv.ClientMsgHandler
	exchangeToken    wsSv.ExchangeToken

	// Repository
	activeConnRepo repo.ActiveConnStore

	// Queue (Valkey-backed, used by long-polling transport)
	queueAdapter queue.Adapter

	// WebSocket server (gobwas)
	gobwasServer wsSv.WebsocketServer

	// Event trigger
	onNewStuff   wsSv.OnNewStuffFn
	onCloseStuff wsSv.OnCloseStuffFn
}

// New creates a new ServerDelivery implementation
func New(
	c configprovider.ConfigStore,
	wpool *workerpool.WorkerPool,
	healthy healthyprovider.Healthy,
	wsService wsSv.WsService,
	connectionMgr wsSv.ConnectionManager,
	rateLimiter wsSv.RateLimiter,
	clientMsgHandler wsSv.ClientMsgHandler,
	exchangeToken wsSv.ExchangeToken,
	repo repo.AllRepository,
	queueAdapter queue.Adapter,
	onNewStuff wsSv.OnNewStuffFn,
	onCloseStuff wsSv.OnCloseStuffFn,
) wsSv.ServerDelivery {
	ins := &serverDelivery{
		c:                c,
		mux:              http.NewServeMux(),
		workerPool:       wpool,
		wsService:        wsService,
		connectionMgr:    connectionMgr,
		rateLimiter:      rateLimiter,
		clientMsgHandler: clientMsgHandler,
		exchangeToken:    exchangeToken,
		activeConnRepo:   repo.ActiveConnStore(),
		queueAdapter:     queueAdapter,
		onNewStuff:       onNewStuff,
		onCloseStuff:     onCloseStuff,
	}

	ins.onCloseRegister()
	ins.onNewRegister()
	// Create gobwas WebSocket server with callbacks
	ins.gobwasServer = gobwas.NewServer(
		c,
		wpool,
		healthy,
		ins.onTextMessage(),
		ins.onBinMessage(),
		ins.onReadError(),
		ins.onWriteError(),
		onCloseStuff,
	)

	// Register HTTP handlers
	ins.registerHandlers()

	return ins
}

// Mux implements ServerDelivery interface
func (d *serverDelivery) Mux() *http.ServeMux {
	return d.mux
}

// registerHandlers registers all HTTP handlers
func (d *serverDelivery) registerHandlers() {
	// POST /issue-tmp-token - Issue temporary connection token
	d.mux.Handle("POST /issue-tmp-token", d.IssueTmpToken())

	// gw - WebSocket endpoint (gobwas)
	d.mux.Handle("/gw", d.GobwasEndpoint())

	// lp - Long polling endpoints (fallback when WebSocket is not supported)
	d.mux.Handle("POST /lp", d.LongPollingEndpoint())
	d.mux.Handle("POST /lp-send", d.LongPollingSendEndpoint())
}

// Callback functions for WebSocket server

func (d *serverDelivery) onTextMessage() wsSv.OnTextMessageFn {
	return func(payload string, auth voAuth.WebsocketAuth, sendFn func([]byte)) {
		d.clientMsgHandler.HandleTextMessage(payload, auth, sendFn)
	}
}

func (d *serverDelivery) onBinMessage() wsSv.OnBinMessageFn {
	return func(payload []byte, auth voAuth.WebsocketAuth, sendFn func([]byte)) {
		d.clientMsgHandler.HandleBinMessage(payload, auth, sendFn)
	}
}

func (d *serverDelivery) onReadError() wsSv.OnReadErrorFn {
	return func(auth voAuth.WebsocketAuth, err error) {
		slog.Error("WebSocket read error", slog.Any("auth", auth), slog.Any("error", err))
	}
}

func (d *serverDelivery) onWriteError() wsSv.OnWriteErrorFn {
	return func(auth voAuth.WebsocketAuth, err error) {
		slog.Error("WebSocket write error", slog.Any("auth", auth), slog.Any("error", err))
	}
}

func (d *serverDelivery) onCloseRegister() {
	d.onCloseStuff.RegisterAll(func(auth voAuth.WebsocketAuth) {
		// Clean up in-memory state
		d.connectionMgr.RemoveConnection(auth)
		d.rateLimiter.Remove(auth)

		aErr := d.activeConnRepo.RemoveConnection(context.Background(), auth.UserID, auth.InstanceID)
		if aErr != nil {
			slog.Error("Failed to remove connection from DynamoDB",
				slog.Any("error", aErr),
				slog.Any("auth", auth))
		}
	})
}

// onNew handles new WebSocket connection
func (d *serverDelivery) onNewRegister() {
	d.onNewStuff.Register(
		wsSv.OnNewWsKeyName("NewConnection"),
		func(connection wsSv.WebsocketConn) error {
			auth := connection.Auth()
			ctx := context.Background()

			// Check for duplicate connection in memory
			existingConn, ok := d.connectionMgr.GetConnection(connection.Auth())
			if ok {
				slog.Warn("Duplicate connection detected, closing old connection")
				existingConn.Close()
				time.Sleep(time.Millisecond * 500) // Wait for old connection to close
			}

			// Persist connection to DynamoDB
			aErr := d.activeConnRepo.AddConnection(ctx, auth.UserID, auth.InstanceID)
			if aErr != nil {
				return aErr
			}

			// Add to in-memory manager
			d.connectionMgr.AddConnection(connection)
			d.rateLimiter.New(connection.Auth())

			return nil
		})

	fns := d.c.Env().Fns
	if fns != nil && fns.OnNewConnection != nil {
		d.onNewStuff.Register(
			wsSv.OnNewWsKeyName("FnsProvided"),
			func(connection wsSv.WebsocketConn) error {
				return fns.OnNewConnection.OnNewConnection(context.Background(), connection.Auth())
			})
	}
}
