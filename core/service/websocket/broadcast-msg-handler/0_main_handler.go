package broadcastmsghandler

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	ackmanager "github.com/pipewave-dev/go-pkg/core/service/websocket/ack-manager"
)

type broadcastMsgHandler struct {
	storeActiveWs repo.ActiveConnStore
	connections   wsSv.ConnectionManager
	ackManager    *ackmanager.AckManager
}

func New(
	repo repository.AllRepository,
	connections wsSv.ConnectionManager,
	ackMgr *ackmanager.AckManager,
) broadcast.PubsubHandler {
	return &broadcastMsgHandler{
		storeActiveWs: repo.ActiveConnStore(),
		connections:   connections,
		ackManager:    ackMgr,
	}
}
