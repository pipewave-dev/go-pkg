package clientmsghandler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/wire"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	metricsprovider "github.com/pipewave-dev/go-pkg/provider/metrics-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
	"github.com/samber/do/v2"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	ackmanager "github.com/pipewave-dev/go-pkg/core/service/websocket/ack-manager"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	otelP "github.com/pipewave-dev/go-pkg/pkg/otel"
)

func NewDI(i do.Injector) (wsSv.ClientMsgHandler, error) {
	allRepo := do.MustInvoke[repo.AllRepository](i)
	obs := do.MustInvoke[observer.Observability](i)
	pubsubAdapter := do.MustInvoke[pubsub.Adapter](i)
	otelProvider := do.MustInvoke[otelP.OtelProvider](i)
	rateLimiter := do.MustInvoke[wsSv.RateLimiter](i)
	ackMgr := do.MustInvoke[*ackmanager.AckManager](i)
	metricsProvider := do.MustInvoke[*metricsprovider.Provider](i)
	connectionMgr := do.MustInvoke[wsSv.ConnectionManager](i)

	return &clientMsgHandler{
		c:             do.MustInvoke[configprovider.ConfigStore](i),
		obs:           obs,
		pubsubAdapter: pubsubAdapter,
		otelProvider:  otelProvider,
		broadcast:     broadcast.NewMsgCreator(do.MustInvoke[configprovider.ConfigStore](i), pubsubAdapter, otelProvider, do.MustInvoke[fncollector.CleanupTask](i)),
		rateLimiter:   rateLimiter,
		activeConn:    allRepo.ActiveConnStore(),
		user:          allRepo.User(),
		hbThrottle:    newHeartbeatThrottle(do.MustInvoke[fncollector.IntervalTask](i)),
		deduplicator:  newMsgDeduplicator(do.MustInvoke[fncollector.IntervalTask](i)),
		ackManager:    ackMgr,
		metrics:       metricsProvider.Metrics(),
		connectionMgr: connectionMgr,
	}, nil
}

type clientMsgHandler struct {
	c             configprovider.ConfigStore
	obs           observer.Observability
	pubsubAdapter pubsub.Adapter
	otelProvider  otelP.OtelProvider
	broadcast     broadcast.MsgCreator
	rateLimiter   wsSv.RateLimiter
	activeConn    repo.ActiveConnStore
	user          repo.User
	hbThrottle    *heartbeatThrottle
	deduplicator  *msgDeduplicator
	ackManager    *ackmanager.AckManager
	metrics       *metrics.PipewaveMetrics
	connectionMgr wsSv.ConnectionManager
}

var hearbeatResMsg = wsSv.WebsocketResponse{
	MsgType: wsSv.MessageTypeHeartbeat,
	Binary:  nil,
}

func (h *clientMsgHandler) HandleTextMessage(ctx context.Context, clientMsg string, auth voAuth.WebsocketAuth, sendFn func(context.Context, []byte) error) {
	slog.ErrorContext(ctx, "Text message isn't supported")
}

func (h *clientMsgHandler) HandleBinMessage(ctx context.Context, clientMsg []byte, auth voAuth.WebsocketAuth, sendFn func(context.Context, []byte) error) {
	h.handleMessage(ctx, clientMsg, auth, sendFn)
}

func (h *clientMsgHandler) handleMessage(ctx context.Context, clientMsg []byte, auth voAuth.WebsocketAuth, sendFn func(context.Context, []byte) error) {
	var response *wsSv.WebsocketResponse

	// Instrument every exit path from one place: a defer cannot miss a branch
	// the way scattered Record calls can.
	start := time.Now()
	rawMsgType := ""
	outcome := metrics.OutcomeOK
	defer func() {
		h.metrics.RecordClientMessage(ctx, rawMsgType, outcome, time.Since(start).Seconds())
	}()

	defer func() {
		if response != nil {
			data := response.Marshall()
			if data == nil {
				// Marshall returns nil only when a field exceeds its wire
				// limit — a server bug (an over-long field slipped through
				// validation), not a transient failure. Surface it instead
				// of silently sending a zero-length frame to the client.
				slog.WarnContext(ctx, "Dropping response: Marshall returned nil (field exceeds wire limit)",
					slog.String("msgType", string(response.MsgType)))
				return
			}
			sendFn(ctx, data)
		}
	}()

	var msg wsSv.WebsocketResquest
	err2 := msg.Unmarshall(clientMsg)
	if err2 != nil {
		// Invalid message format
		outcome = metrics.OutcomeInvalidSchema
		response = &wsSv.WebsocketResponse{
			MsgType: wsSv.MessageTypeError,
			Error:   aerror.New(ctx, aerror.InvalidInputSchema, err2).Error(),
		}

		if errors.Is(err2, wire.ErrVersion) {
			// A version mismatch can't be fixed by retrying the same frame —
			// the peer is speaking a different wire version. Close the
			// connection with a distinct code/reason instead of sending an
			// error frame the peer may not even be able to decode, so
			// on-call gets a diagnosable signal instead of a silent
			// reconnect loop indistinguishable from a generic decode error.
			h.closeOnVersionMismatch(ctx, auth)
			response = nil
		}
		return
	}
	rawMsgType = string(msg.MsgType)

	switch msg.MsgType {
	case wsSv.MessageTypeHeartbeat:
		if !h.rateLimiter.Get(auth).Allow() {
			outcome = metrics.OutcomeRateLimited
			return
		}
		h.handleHeartbeat(ctx, auth)
		response = &hearbeatResMsg

	case wsSv.MessageTypeAck:
		if !h.rateLimiter.Get(auth).Allow() {
			outcome = metrics.OutcomeRateLimited
			return
		}
		// Handle ACK from client
		ackID := string(msg.Binary)
		if ackID == "" {
			return
		}
		if h.ackManager.ResolveAck(ackID) {
			return
		}
		// Not a local ack — route back to the originating container
		if sourceContainerID, ok := h.ackManager.ResolveRemoteAck(ackID); ok {
			if err := h.broadcast.AckResolved(ctx, []string{sourceContainerID}, broadcast.AckResolvedParams{AckID: ackID}).Publish(); err != nil {
				slog.WarnContext(ctx, "Failed to publish AckResolved",
					slog.String("ackID", ackID),
					slog.String("sourceContainerID", sourceContainerID),
					slog.Any("error", err))
			}
		}
		return // No response needed

	default:
		resID := fn.NewUUID()
		rl := h.rateLimiter.Get(auth)
		if !rl.Allow() {
			outcome = metrics.OutcomeRateLimited
			response = &wsSv.WebsocketResponse{
				Id:           resID.String(),
				ResponseToId: msg.Id,
				MsgType:      msg.MsgType,
				Error:        aerror.New(ctx, aerror.RateLimitExceeded, nil).Error(),
			}
			return
		}

		if msg.Id != "" && h.deduplicator.isDuplicate(msg.Id+auth.InstanceID) {
			outcome = metrics.OutcomeDedup
			return
		}

		msgType, res, err := h.c.Env().Fns.HandleMessage.HandleMessage(ctx, auth, string(msg.MsgType), msg.Binary)
		if err != nil {
			outcome = metrics.OutcomeError
			response = &wsSv.WebsocketResponse{
				Id:           resID.String(),
				ResponseToId: msg.Id,
				MsgType:      msg.MsgType,
				Error:        err.Error(),
			}
		} else {
			if msgType == "" {
				return
			}
			response = &wsSv.WebsocketResponse{
				Id:           resID.String(),
				ResponseToId: msg.Id,
				MsgType:      wsSv.MessageType(msgType),
				Binary:       res,
			}
		}
	}
}

// closeOnVersionMismatch closes the connection identified by auth with a
// dedicated close code/reason naming the wire-protocol version mismatch, so
// on-call can diagnose it instead of seeing a generic decode failure and a
// silent client reconnect loop.
//
// Close code 1002 (protocol error) is reused here rather than minting a
// private code: it is already the code this package uses elsewhere for
// protocol violations (see server/gobwas handleProtocolError), so a single
// well-known code stays consistent across all "the peer broke the wire
// protocol" cases; the reason string is what actually distinguishes this
// case for on-call/log search.
func (h *clientMsgHandler) closeOnVersionMismatch(ctx context.Context, auth voAuth.WebsocketAuth) {
	const (
		versionMismatchCloseCode   = 1002 // ws.StatusProtocolError
		versionMismatchCloseReason = "wire protocol version mismatch"
	)

	conn, ok := h.connectionMgr.GetConnection(auth)
	if !ok {
		slog.ErrorContext(ctx, "Wire protocol version mismatch: could not find connection to close",
			slog.Any("auth", auth))
		return
	}

	closer, ok := conn.(wsSv.CloseWithReasonConn)
	if !ok {
		// Transport doesn't support a coded close (e.g. long polling, where
		// "version mismatch" doesn't map to a close-frame concept). Fail
		// loudly instead of silently dropping the frame.
		//
		// TODO: if a non-WebSocket transport ever needs a diagnosable
		// version-mismatch signal too, it needs its own out-of-band
		// mechanism (e.g. a distinguishable HTTP status/body on /lp-send),
		// since it has no close-frame handshake to piggyback on.
		slog.ErrorContext(ctx, "Wire protocol version mismatch: connection does not support coded close",
			slog.Any("auth", auth))
		return
	}

	slog.ErrorContext(ctx, "Wire protocol version mismatch: closing connection",
		slog.Any("auth", auth),
		slog.Int("closeCode", versionMismatchCloseCode))
	closer.CloseWithReason(versionMismatchCloseCode, versionMismatchCloseReason)
}

func (h *clientMsgHandler) handleHeartbeat(ctx context.Context, auth voAuth.WebsocketAuth) {
	// Throttle per-session: prevents duplicate writes when a connection sends
	// heartbeats faster than heartbeatThrottleDuration.
	if h.hbThrottle.shouldUpdate("s:" + auth.InstanceID) {
		aErr := h.activeConn.UpdateHeartBeat(ctx, auth.UserID, auth.InstanceID)
		if aErr != nil {
			slog.Warn("Failed to update heartbeat", slog.Any("error", aErr), slog.Any("auth", auth))
		}
	}

	// Throttle per-user: collapses writes from all tabs/devices of the same user
	// into at most 1 DynamoDB write per heartbeatThrottleDuration.
	if h.hbThrottle.shouldUpdate("u:" + auth.UserID) {
		aErr := h.user.UpdateLastHeartbeat(ctx, auth.UserID)
		if aErr != nil {
			slog.Warn("Failed to update heartbeat", slog.Any("error", aErr), slog.Any("auth", auth))
		}
	}
}
