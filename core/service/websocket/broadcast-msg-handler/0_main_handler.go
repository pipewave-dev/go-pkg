package broadcastmsghandler

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	ackmanager "github.com/pipewave-dev/go-pkg/core/service/websocket/ack-manager"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	msghub "github.com/pipewave-dev/go-pkg/core/service/websocket/msg-hub"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
)

type broadcastMsgHandler struct {
	storeActiveWs repo.ActiveConnStore
	connections   wsSv.ConnectionManager
	ackManager    *ackmanager.AckManager
	msgHubSvc     msghub.MessageHubSvc

	wp *workerpool.WorkerPool
}

func New(
	repo repository.AllRepository,
	connections wsSv.ConnectionManager,
	ackMgr *ackmanager.AckManager,
	msgHubSvc msghub.MessageHubSvc,
	wp *workerpool.WorkerPool,
) broadcast.PubsubHandler {
	return &broadcastMsgHandler{
		storeActiveWs: repo.ActiveConnStore(),
		connections:   connections,
		ackManager:    ackMgr,
		msgHubSvc:     msgHubSvc,
		wp:            wp,
	}
}
