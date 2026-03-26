package clientmsghandler

import (
	"context"
	"fmt"
	"log/slog"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	"github.com/pipewave-dev/go-pkg/shared/actx"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/pipewave-dev/go-pkg/shared/utils/fn"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	ackmanager "github.com/pipewave-dev/go-pkg/core/service/websocket/ack-manager"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	otelP "github.com/pipewave-dev/go-pkg/pkg/otel"
)

type clientMsgHandler struct {
	c             configprovider.ConfigStore
	cleanupTask   fncollector.CleanupTask
	intervalTask  fncollector.IntervalTask
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
}

func New(
	c configprovider.ConfigStore,
	cleanupTask fncollector.CleanupTask,
	intervalTask fncollector.IntervalTask,
	obs observer.Observability,
	pubsubAdapter pubsub.Adapter,
	otelProvider otelP.OtelProvider,
	rateLimiter wsSv.RateLimiter,
	repo repo.AllRepository,
	ackMgr *ackmanager.AckManager,
) wsSv.ClientMsgHandler {
	return &clientMsgHandler{
		c:             c,
		obs:           obs,
		pubsubAdapter: pubsubAdapter,
		otelProvider:  otelProvider,
		broadcast:     broadcast.NewMsgCreator(c, pubsubAdapter, otelProvider, cleanupTask),
		rateLimiter:   rateLimiter,
		activeConn:    repo.ActiveConnStore(),
		user:          repo.User(),
		hbThrottle:    newHeartbeatThrottle(intervalTask),
		deduplicator:  newMsgDeduplicator(intervalTask),
		ackManager:    ackMgr,
	}
}

var (
	hearbeatResMsg = wsSv.WebsocketResponse{
		MsgType: wsSv.MessageTypeHeartbeat,
		Binary:  nil,
	}
	lenHearbeat = len((&wsSv.WebsocketResquest{
		MsgType: wsSv.MessageTypeHeartbeat,
		Binary:  nil,
	}).Marshall())
)

func (h *clientMsgHandler) HandleTextMessage(clientMsg string, auth voAuth.WebsocketAuth, sendFn func([]byte)) {
	msg := fmt.Sprintf("Your UserID: %s, send msg: %s", auth.UserID, clientMsg)
	sendFn([]byte(msg))
}

func (h *clientMsgHandler) HandleBinMessage(clientMsg []byte, auth voAuth.WebsocketAuth, sendFn func([]byte)) {
	h.handleMessage(clientMsg, auth, sendFn)
}

func (h *clientMsgHandler) handleMessage(clientMsg []byte, auth voAuth.WebsocketAuth, sendFn func([]byte)) {
	var response *wsSv.WebsocketResponse
	aCtx := actx.From(context.Background())

	aCtx.SetAuth(
		voAuth.UserAuth(auth.UserID, auth.InstanceID, false))
	aCtx.SetTraceID("wsmsg" + fn.NewNanoID(18))

	defer func() {
		if response != nil {
			data := response.Marshall()
			sendFn(data)
		}
	}()

	var msg wsSv.WebsocketResquest
	err2 := msg.Unmarshall(clientMsg)
	if err2 != nil {
		// Invalid message format
		response = &wsSv.WebsocketResponse{
			Error: aerror.New(aCtx, aerror.InvalidInputSchema, err2).Error(),
		}
		return
	}

	switch msg.MsgType {
	case wsSv.MessageTypeHeartbeat:
		h.handleHeartbeat(aCtx, auth)
		response = &hearbeatResMsg

	case wsSv.MessageTypeAck:
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
			if err := h.broadcast.AckResolved(aCtx, []string{sourceContainerID}, broadcast.AckResolvedParams{AckID: ackID}).Publish(); err != nil {
				slog.WarnContext(aCtx, "Failed to publish AckResolved",
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
			response = &wsSv.WebsocketResponse{
				Id:           resID.String(),
				ResponseToId: msg.Id,
				MsgType:      msg.MsgType,
				Error:        aerror.New(aCtx, aerror.RateLimitExceeded, nil).Error(),
			}
			return
		}

		if msg.Id != "" && h.deduplicator.isDuplicate(msg.Id+auth.InstanceID) {
			return
		}

		msgType, res, err := h.c.Env().Fns.HandleMessage.HandleMessage(aCtx, auth, string(msg.MsgType), msg.Binary)
		if err != nil {
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
