package clientmsghandler

import (
	"context"
	"log/slog"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
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

	defer func() {
		if response != nil {
			data := response.Marshall()
			sendFn(ctx, data)
		}
	}()

	var msg wsSv.WebsocketResquest
	err2 := msg.Unmarshall(clientMsg)
	if err2 != nil {
		// Invalid message format
		response = &wsSv.WebsocketResponse{
			Error: aerror.New(ctx, aerror.InvalidInputSchema, err2).Error(),
		}
		return
	}

	switch msg.MsgType {
	case wsSv.MessageTypeHeartbeat:
		h.handleHeartbeat(ctx, auth)
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
			response = &wsSv.WebsocketResponse{
				Id:           resID.String(),
				ResponseToId: msg.Id,
				MsgType:      msg.MsgType,
				Error:        aerror.New(ctx, aerror.RateLimitExceeded, nil).Error(),
			}
			return
		}

		if msg.Id != "" && h.deduplicator.isDuplicate(msg.Id+auth.InstanceID) {
			return
		}

		msgType, res, err := h.c.Env().Fns.HandleMessage.HandleMessage(ctx, auth, string(msg.MsgType), msg.Binary)
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
