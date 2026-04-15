package delivery

import (
	"context"
	"log/slog"
	"net/http"

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
	"github.com/pipewave-dev/go-pkg/shared/actx"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (wsSv.ServerDelivery, error) {
	c := do.MustInvoke[configprovider.ConfigStore](i)
	wpool := do.MustInvoke[*workerpool.WorkerPool](i)
	healthy := do.MustInvoke[healthyprovider.Healthy](i)
	wsService := do.MustInvoke[wsSv.WsService](i)
	connectionMgr := do.MustInvoke[wsSv.ConnectionManager](i)
	rateLimiter := do.MustInvoke[wsSv.RateLimiter](i)
	clientMsgHandler := do.MustInvoke[wsSv.ClientMsgHandler](i)
	exchangeToken := do.MustInvoke[wsSv.ExchangeToken](i)
	repo := do.MustInvoke[repo.AllRepository](i)
	queueAdapter := do.MustInvoke[queue.Adapter](i)
	onNewStuff := do.MustInvoke[wsSv.OnNewStuffFn](i)
	onCloseStuff := do.MustInvoke[wsSv.OnCloseStuffFn](i)
	msgHubSvc := do.MustInvoke[msghub.MessageHubSvc](i)
	shutdownSignal := do.MustInvoke[*msghub.ShutdownSignal](i)

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
		gobwasServer:     nil,
	}

	ins.registerCallback()
	// Create gobwas WebSocket server with callbacks.
	// OnReadError and OnWriteError are wrapped lazily because Fns is injected via
	// SetFns() after NewPipewave() returns, so c.Env().Fns is nil at this point.
	ins.gobwasServer = gobwas.NewServer(
		c,
		wpool,
		healthy,
		ins.clientMsgHandler.HandleTextMessage,
		ins.clientMsgHandler.HandleBinMessage,
		wsSv.OnReadErrorFn(func(ctx context.Context, auth voAuth.WebsocketAuth, err error) {
			if fns := c.Env().Fns; fns != nil && fns.OnReadError != nil {
				fns.OnReadError.OnReadError(ctx, auth, err)
			}
		}),
		wsSv.OnWriteErrorFn(func(ctx context.Context, auth voAuth.WebsocketAuth, err error) {
			if fns := c.Env().Fns; fns != nil && fns.OnWriteError != nil {
				fns.OnWriteError.OnWriteError(ctx, auth, err)
			}
		}),
		onCloseStuff,
	)

	// Register HTTP handlers
	ins.registerHandlers()

	return ins, nil
}

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

// registerCallback handles new WebSocket connection
func (d *serverDelivery) registerCallback() {
	d.onNewStuff.Register(
		wsSv.OnNewWsKeyName("NewConnection"),
		func(connection wsSv.WebsocketConn) error {
			auth := connection.Auth()
			ctx := actx.New()
			ctx.SetWebsocketAuth(auth)

			// Close stale in-memory duplicate.
			if existingConn, ok := d.connectionMgr.GetConnection(auth); ok {
				slog.Warn("Duplicate connection detected, closing old connection")
				existingConn.Close()
			}

			// Check previous session state for reconnect handling.
			actConn, aErr := d.activeConnRepo.GetInstanceConnection(ctx, auth.UserID, auth.InstanceID)
			if aErr == nil && actConn != nil {
				switch actConn.Status {
				case voWs.WsStatusConnected:
					// Stale duplicate: signal old container to disconnect immediately.
					d.wsService.DisconnectSession(ctx, actConn.UserID, auth.InstanceID)

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
					slog.DebugContext(ctx, "onNew: reconnect after container shutdown (WsStatusTransferring)",
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
					if err := dc.SendDirect(ctx, msg); err != nil {
						slog.ErrorContext(ctx, "onNew: SendDirect failed for pending message",
							slog.String("userID", auth.UserID),
							slog.String("instanceID", auth.InstanceID),
							slog.Any("error", err))
					}
				} else {
					// Fallback: connection does not implement DrainableConn.
					if err := connection.Send(ctx, msg); err != nil {
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

	d.onCloseStuff.RegisterAll(func(auth voAuth.WebsocketAuth) {
		d.connectionMgr.RemoveConnection(auth)
		d.rateLimiter.Remove(auth)

		ctx := actx.New()
		ctx.SetWebsocketAuth(auth)

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
			d.msgHubSvc.DeleteAllPendingMessage(ctx, auth.UserID, auth.InstanceID) // clean up pending messages on session expiration
		})
	})
}
