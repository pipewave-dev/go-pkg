package mediatorsvc

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	ackmanager "github.com/pipewave-dev/go-pkg/core/service/websocket/ack-manager"
	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	otelP "github.com/pipewave-dev/go-pkg/pkg/otel"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
)

type mediatorSvc struct {
	activeConnRepo   repo.ActiveConnStore
	cleanupTask      fncollector.CleanupTask
	wpool            *workerpool.WorkerPool
	connections      wsSv.ConnectionManager
	broadcastHandler br.PubsubHandler
	broadcast        br.MsgCreator
	ackManager       *ackmanager.AckManager
}

func New(
	repo repository.AllRepository,
	cleanupTask fncollector.CleanupTask,
	wpool *workerpool.WorkerPool,
	connections wsSv.ConnectionManager,
	broadcastHandler br.PubsubHandler,
	pubsubAdapter pubsub.Adapter,
	otelProvider otelP.OtelProvider,
	ackMgr *ackmanager.AckManager,
) wsSv.WsService {
	ins := &mediatorSvc{
		activeConnRepo:   repo.ActiveConnStore(),
		cleanupTask:      cleanupTask,
		wpool:            wpool,
		connections:      connections,
		broadcastHandler: broadcastHandler,

		broadcast:  br.NewMsgCreator(pubsubAdapter, otelProvider, cleanupTask),
		ackManager: ackMgr,
	}

	br.StartSubscribers(ins.broadcastHandler, pubsubAdapter, otelProvider, cleanupTask)

	cleanupTask.RegTask(ins.Shutdown, fncollector.FnPriorityLate)

	return ins
}
