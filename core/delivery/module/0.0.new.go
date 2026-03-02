package moduledelivery

import (
	"net/http"
	"time"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	business "github.com/pipewave-dev/go-pkg/core/service/business"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	mm "github.com/pipewave-dev/go-pkg/pkg/mux-middleware"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	healthyprovider "github.com/pipewave-dev/go-pkg/provider/healthy-provider"
)

type moduleDelivery struct {
	c configprovider.ConfigStore

	mux *http.ServeMux

	mw mm.MiddlewareProvider

	wsDeli       wsSv.ServerDelivery
	wsService    wsSv.WsService
	wsOnNewReg   wsSv.OnNewStuffFn
	wsOnCloseReg wsSv.OnCloseStuffFn

	healthy       healthyprovider.Healthy
	monitoringSvc business.Monitoring

	workerPool   *workerpool.WorkerPool
	cleanupTask  fncollector.CleanupTask
	intervalTask fncollector.IntervalTask
}

func New(
	c configprovider.ConfigStore,
	mw mm.MiddlewareProvider,
	wsDeli wsSv.ServerDelivery,
	wsService wsSv.WsService,
	wsOnNewReg wsSv.OnNewStuffFn,
	wsOnCloseReg wsSv.OnCloseStuffFn,
	healthy healthyprovider.Healthy,
	monitoringSvc business.Monitoring,
	workerPool *workerpool.WorkerPool,
	cleanupTask fncollector.CleanupTask,
	intervalTask fncollector.IntervalTask,
) delivery.ModuleDelivery {
	ins := &moduleDelivery{
		c:             c,
		mux:           http.NewServeMux(),
		mw:            mw,
		wsDeli:        wsDeli,
		wsService:     wsService,
		wsOnNewReg:    wsOnNewReg,
		wsOnCloseReg:  wsOnCloseReg,
		healthy:       healthy,
		monitoringSvc: monitoringSvc,

		workerPool:   workerPool,
		cleanupTask:  cleanupTask,
		intervalTask: intervalTask,
	}
	ins.registerHandlers()
	stopFn := ins.runIntervalTasks(time.Second * 600)
	cleanupTask.RegTask(stopFn, fncollector.FnPriorityEarlyest) // Stop to prevent push new tasks
	return ins
}

func (m *moduleDelivery) runIntervalTasks(d time.Duration) func() {
	ticker := time.NewTicker(d)
	done := make(chan struct{})

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fns := m.intervalTask.Get()
				m.workerPool.Submit(func() {
					for _, fn := range fns {
						fn()
					}
				})
			case <-done:
				return
			}
		}
	}()

	return func() {
		close(done)
	}
}

func (m *moduleDelivery) Shutdown() {
	m.healthy.SetUnhealthy("Shutting down")
	fns := m.cleanupTask.Get()
	for _, fn := range fns {
		fn()
	}
}

func (m *moduleDelivery) IsHealthy() bool {
	return m.healthy.IsHealthy()
}
