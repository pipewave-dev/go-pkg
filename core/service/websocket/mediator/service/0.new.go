package mediatorsvc

import (
	"fmt"
	"time"

	"github.com/pipewave-dev/go-pkg/core/repository"
	repo "github.com/pipewave-dev/go-pkg/core/repository"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	ackmanager "github.com/pipewave-dev/go-pkg/core/service/websocket/ack-manager"
	br "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast"
	msghub "github.com/pipewave-dev/go-pkg/core/service/websocket/msg-hub"
	otelP "github.com/pipewave-dev/go-pkg/pkg/otel"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (wsSv.WsService, error) {
	c := do.MustInvoke[configprovider.ConfigStore](i)
	repo := do.MustInvoke[repository.AllRepository](i)
	cleanupTask := do.MustInvoke[fncollector.CleanupTask](i)
	wpool := do.MustInvoke[*workerpool.WorkerPool](i)
	connections := do.MustInvoke[wsSv.ConnectionManager](i)
	broadcastHandler := do.MustInvoke[br.PubsubHandler](i)
	pubsubAdapter := do.MustInvoke[pubsub.Adapter](i)
	otelProvider := do.MustInvoke[otelP.OtelProvider](i)
	ackMgr := do.MustInvoke[*ackmanager.AckManager](i)
	msgHubSvc := do.MustInvoke[msghub.MessageHubSvc](i)
	shutdownSignal := do.MustInvoke[*msghub.ShutdownSignal](i)

	ins := &mediatorSvc{
		c:                c,
		activeConnRepo:   repo.ActiveConnStore(),
		cleanupTask:      cleanupTask,
		wpool:            wpool,
		connections:      connections,
		broadcastHandler: broadcastHandler,

		broadcast:      br.NewMsgCreator(c, pubsubAdapter, otelProvider, cleanupTask),
		ackManager:     ackMgr,
		msgHubSvc:      msgHubSvc,
		shutdownSignal: shutdownSignal,
	}

	br.StartSubscribers(ins.broadcastHandler, c, pubsubAdapter, otelProvider, cleanupTask)

	stopPingLoop := ins.startPingLoop()
	cleanupTask.RegTask(stopPingLoop, fncollector.FnPriorityEarlyest)
	cleanupTask.RegTask(ins.Shutdown, fncollector.FnPriorityNormal)
	// Report result, should occur after all cleanup tasks are done.
	cleanupTask.RegTask(func() {
		time.Sleep(3 * time.Second) // allow some time for reconnects before declaring shutdown complete
		ins.checkTransferingConns()
	}, fncollector.FnPriorityLatest)

	return ins, nil
}

type mediatorSvc struct {
	c configprovider.ConfigStore

	activeConnRepo   repo.ActiveConnStore
	cleanupTask      fncollector.CleanupTask
	wpool            *workerpool.WorkerPool
	connections      wsSv.ConnectionManager
	broadcastHandler br.PubsubHandler
	broadcast        br.MsgCreator
	ackManager       *ackmanager.AckManager
	msgHubSvc        msghub.MessageHubSvc
	shutdownSignal   *msghub.ShutdownSignal

	// tmp fields for shutdown logic
	transferingConns []connectionInfo
}

type connectionInfo struct {
	userID     string
	instanceID string
}

func (c *connectionInfo) String() string {
	return fmt.Sprintf("%s@%s", c.userID, c.instanceID)
}
