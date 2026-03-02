package mediatorsvc

import (
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	otelP "github.com/pipewave-dev/go-pkg/pkg/otel"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
)

type mediatorSvc struct {
	cleanupTask      fncollector.CleanupTask
	wpool            *workerpool.WorkerPool
	connections      wsSv.ConnectionManager
	broadcastHandler br.PubsubHandler
	broadcast        br.MsgCreator
}

func New(
	cleanupTask fncollector.CleanupTask,
	wpool *workerpool.WorkerPool,
	connections wsSv.ConnectionManager,
	broadcastHandler br.PubsubHandler,
	pubsubAdapter pubsub.Adapter,
	otelProvider otelP.OtelProvider,
) wsSv.WsService {
	ins := &mediatorSvc{
		cleanupTask:      cleanupTask,
		wpool:            wpool,
		connections:      connections,
		broadcastHandler: broadcastHandler,

		broadcast: br.NewMsgCreator(pubsubAdapter, otelProvider, cleanupTask),
	}

	br.StartSubscribers(ins.broadcastHandler, pubsubAdapter, otelProvider, cleanupTask)

	cleanupTask.RegTask(ins.Shutdown, fncollector.FnPriorityLate)

	return ins
}
