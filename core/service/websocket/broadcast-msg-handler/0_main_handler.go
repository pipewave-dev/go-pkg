package broadcastmsghandler

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	ackmanager "github.com/pipewave-dev/go-pkg/core/service/websocket/ack-manager"
	"github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	msghub "github.com/pipewave-dev/go-pkg/core/service/websocket/msg-hub"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (broadcast.PubsubHandler, error) {
	allRepo := do.MustInvoke[repository.AllRepository](i)
	return &broadcastMsgHandler{
		storeActiveWs: allRepo.ActiveConnStore(),
		connections:   do.MustInvoke[wsSv.ConnectionManager](i),
		ackManager:    do.MustInvoke[*ackmanager.AckManager](i),
		msgHubSvc:     do.MustInvoke[msghub.MessageHubSvc](i),
		wp:            do.MustInvoke[*workerpool.WorkerPool](i),
	}, nil
}

type broadcastMsgHandler struct {
	storeActiveWs repo.ActiveConnStore
	connections   wsSv.ConnectionManager
	ackManager    *ackmanager.AckManager
	msgHubSvc     msghub.MessageHubSvc

	wp *workerpool.WorkerPool
}
