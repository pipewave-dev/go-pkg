package moduledelivery

import (
	"context"
	"log/slog"
	"time"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	"github.com/pipewave-dev/go-pkg/core/repository"
	business "github.com/pipewave-dev/go-pkg/core/service/business"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (d *moduleDelivery) Services() delivery.ExportedServices {
	return &getServices{
		wsService:    d.wsService,
		wsOnNewReg:   d.wsOnNewReg,
		wsOnCloseReg: d.wsOnCloseReg,
	}
}

func (d *moduleDelivery) Monitoring() business.Monitoring {
	return d.monitoringSvc
}

type getServices struct {
	wsService    wsSv.WsService
	wsOnNewReg   wsSv.OnNewStuffFn
	wsOnCloseReg wsSv.OnCloseStuffFn

	repo repository.AllRepository
}

func (g *getServices) CheckOnline(ctx context.Context, userID string) (isOnline bool, aErr aerror.AError) {
	return g.wsService.CheckOnline(ctx, userID)
}

func (g *getServices) OnNewRegister() wsSv.OnNewStuffFn {
	return g.wsOnNewReg
}

func (g *getServices) OnCloseRegister() wsSv.OnCloseStuffFn {
	return g.wsOnCloseReg
}

func (g *getServices) SendToSession(ctx context.Context, userID string, instanceID string, msgType string, payload []byte) aerror.AError {
	return g.wsService.SendToSession(ctx, userID, instanceID, msgType, payload)
}

func (g *getServices) SendToUser(ctx context.Context, userID string, msgType string, payload []byte) aerror.AError {
	return g.wsService.SendToUser(ctx, userID, msgType, payload)
}

func (g *getServices) SendToAnonymous(ctx context.Context, msgType string, payload []byte, isSendAll bool, instanceID []string) aerror.AError {
	return g.wsService.SendToAnonymous(ctx, msgType, payload, isSendAll, instanceID)
}

func (g *getServices) SendToAuthenticated(ctx context.Context, msgType string, payload []byte) aerror.AError {
	return g.wsService.SendToAuthenticated(ctx, msgType, payload)
}

func (g *getServices) SendToAll(ctx context.Context, msgType string, payload []byte) aerror.AError {
	return g.wsService.SendToAll(ctx, msgType, payload)
}

func (g *getServices) DisconnectSession(ctx context.Context, userID string, instanceID string) aerror.AError {
	return g.wsService.DisconnectSession(ctx, userID, instanceID)
}

func (g *getServices) DisconnectUser(ctx context.Context, userID string) aerror.AError {
	return g.wsService.DisconnectUser(ctx, userID)
}

func (g *getServices) SendToUsers(ctx context.Context, userIDs []string, msgType string, payload []byte) aerror.AError {
	return g.wsService.SendToUsers(ctx, userIDs, msgType, payload)
}

func (g *getServices) CheckOnlineMultiple(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError) {
	return g.wsService.CheckOnlineMultiple(ctx, userIDs)
}

func (g *getServices) GetUserSessions(ctx context.Context, userID string) ([]delivery.SessionInfo, aerror.AError) {
	return g.wsService.GetUserSessions(ctx, userID)
}

func (g *getServices) SendToSessionWithAck(ctx context.Context, userID string, instanceID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	return g.wsService.SendToSessionWithAck(ctx, userID, instanceID, msgType, payload, timeout)
}

func (g *getServices) SendToUserWithAck(ctx context.Context, userID string, msgType string, payload []byte, timeout time.Duration) (acked bool, aErr aerror.AError) {
	return g.wsService.SendToUserWithAck(ctx, userID, msgType, payload, timeout)
}

func (g *getServices) CleanUp(ctx context.Context) aerror.AError {
	var multiErr aerror.AMultiError

	err1 := g.repo.ActiveConnStore().CleanUpExpiredConnections(ctx)
	err2 := g.repo.PendingMessage().CleanUpExpiredPendingMessages(ctx)

	aerror.Append(multiErr, err1, err2)

	slog.ErrorContext(ctx, "Failed to clean up expired websocket resources",
		slog.Any("activeConnError", err1),
		slog.Any("pendingMessageError", err2),
		slog.Any("error", multiErr))

	return nil
}
