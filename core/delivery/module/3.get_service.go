package moduledelivery

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/delivery"
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

func (g *getServices) PingConnections() {
	g.wsService.PingConnections()
}

func (g *getServices) SendToAnonymous(ctx context.Context, msgType string, payload []byte, isSendAll bool, instanceID []string) aerror.AError {
	return g.wsService.SendToAnonymous(ctx, msgType, payload, isSendAll, instanceID)
}

func (g *getServices) Shutdown() {
	g.wsService.Shutdown()
}
