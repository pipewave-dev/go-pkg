package delivery

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	msghub "github.com/pipewave-dev/go-pkg/core/service/websocket/msg-hub"
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

	msgHubSvc      msghub.MessageHubSvc
	shutdownSignal *msghub.ShutdownSignal
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
	msgHubSvc msghub.MessageHubSvc,
	shutdownSignal *msghub.ShutdownSignal,
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
		msgHubSvc:        msgHubSvc,
		shutdownSignal:   shutdownSignal,
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
	return func(payload string, auth voAuth.WebsocketAuth, sendFn func([]byte) error) {
		d.clientMsgHandler.HandleTextMessage(payload, auth, sendFn)
	}
}

func (d *serverDelivery) onBinMessage() wsSv.OnBinMessageFn {
	return func(payload []byte, auth voAuth.WebsocketAuth, sendFn func([]byte) error) {
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
		d.connectionMgr.RemoveConnection(auth)
		d.rateLimiter.Remove(auth)

		ctx := context.Background()

		// Anonymous sessions: always remove permanently (no reconnect buffering for anon).
		if auth.IsAnonymous() {
			if aErr := d.activeConnRepo.RemoveConnection(ctx, auth.UserID, auth.InstanceID); aErr != nil {
				slog.Error("onClose: failed to remove anonymous connection",
					slog.Any("auth", auth), slog.Any("error", aErr))
			}
			return
		}

		// Graceful shutdown path: Shutdown() already called UpdateStatusTransferring +
		// msgHubSvc.Register for this connection before closing. Skip all DB operations
		// to avoid overwriting the Transferring record.
		if d.shutdownSignal.IsShuttingDown() {
			return
		}

		// Normal temp-disconnect path: keep DB record + HolderID for cross-container routing.
		aErr := d.activeConnRepo.UpdateStatus(ctx, auth.UserID, auth.InstanceID, voWs.WsStatusTempDisconnected)
		if aErr != nil {
			slog.Error("onClose: UpdateStatus failed, falling back to RemoveConnection",
				slog.Any("auth", auth), slog.Any("error", aErr))
			_ = d.activeConnRepo.RemoveConnection(ctx, auth.UserID, auth.InstanceID)
			return
		}

		d.msgHubSvc.Register(auth.UserID, auth.InstanceID, func() {
			// ExpiredTimer fired — session never reconnected within TTL.
			if err := d.activeConnRepo.RemoveConnection(ctx, auth.UserID, auth.InstanceID); err != nil {
				slog.Error("onExpired: failed to remove ActiveConnection",
					slog.String("userID", auth.UserID),
					slog.String("instanceID", auth.InstanceID),
					slog.Any("error", err))
			}
		})
	})
}

// onNewRegister handles new WebSocket connection
func (d *serverDelivery) onNewRegister() {
	d.onNewStuff.Register(
		wsSv.OnNewWsKeyName("NewConnection"),
		func(connection wsSv.WebsocketConn) error {
			auth := connection.Auth()
			ctx := context.Background()

			// Close stale in-memory duplicate.
			if existingConn, ok := d.connectionMgr.GetConnection(auth); ok {
				slog.Warn("Duplicate connection detected, closing old connection")
				existingConn.Close()
				time.Sleep(time.Millisecond * 500)
			}

			// Check previous session state for reconnect handling.
			actConn, aErr := d.activeConnRepo.GetInstanceConnection(ctx, auth.UserID, auth.InstanceID)
			if aErr == nil && actConn != nil {
				switch actConn.Status {
				case voWs.WsStatusTempDisconnected:
					// Normal reconnect: signal old container to cancel its ExpiredTimer.
					if sigErr := d.wsService.ResumeSession(ctx, actConn.HolderID, auth.UserID, auth.InstanceID); sigErr != nil {
						slog.WarnContext(ctx, "onNew: ResumeSession publish failed; old ExpiredTimer will eventually fire",
							slog.String("holderID", actConn.HolderID),
							slog.String("userID", auth.UserID),
							slog.String("instanceID", auth.InstanceID),
							slog.Any("error", sigErr))
					}
				case voWs.WsStatusTransferring:
					// Container-shutdown reconnect: HolderID is empty, old container is shutting down.
					// No signal needed — AddConnection below will claim this session.
					slog.InfoContext(ctx, "onNew: reconnect after container shutdown (WsStatusTransferring)",
						slog.String("userID", auth.UserID),
						slog.String("instanceID", auth.InstanceID))
				}
			}

			// Upsert: updates HolderID to this container + resets Status to WsStatusConnected.
			if aErr = d.activeConnRepo.AddConnection(ctx, auth.UserID, auth.InstanceID, connection.CoreType()); aErr != nil {
				return aErr
			}

			// Begin drain BEFORE registering in ConnectionManager.
			// This blocks concurrent Send() calls (which acquire RLock) until drain is complete,
			// ensuring pending messages are delivered before any new messages.
			if dc, ok := connection.(wsSv.DrainableConn); ok {
				dc.BeginDrain()
				defer dc.EndDrain()
			}

			d.connectionMgr.AddConnection(connection)
			d.rateLimiter.New(auth)

			// Consume buffered pending messages and deliver them via SendDirect
			// (bypasses drainMu to avoid deadlock while WLock is held).
			msgs, consumeErr := d.msgHubSvc.Consume(ctx, auth.UserID, auth.InstanceID)
			if consumeErr != nil {
				slog.WarnContext(ctx, "onNew: failed to consume pending messages; session continues without them",
					slog.String("userID", auth.UserID),
					slog.String("instanceID", auth.InstanceID),
					slog.Any("error", consumeErr))
			}
			for _, msg := range msgs {
				if dc, ok := connection.(wsSv.DrainableConn); ok {
					if err := dc.SendDirect(msg); err != nil {
						slog.ErrorContext(ctx, "onNew: SendDirect failed for pending message",
							slog.String("userID", auth.UserID),
							slog.String("instanceID", auth.InstanceID),
							slog.Any("error", err))
					}
				} else {
					// Fallback: connection does not implement DrainableConn.
					if err := connection.Send(msg); err != nil {
						slog.ErrorContext(ctx, "onNew: Send failed for pending message",
							slog.String("userID", auth.UserID),
							slog.String("instanceID", auth.InstanceID),
							slog.Any("error", err))
					}
				}
			}
			// defer dc.EndDrain() fires here → blocked Send() goroutines proceed after pending messages.

			return nil
		})
}
