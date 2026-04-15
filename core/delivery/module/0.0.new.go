package moduledelivery

import (
	"net/http"
	"time"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	"github.com/pipewave-dev/go-pkg/core/repository"
	business "github.com/pipewave-dev/go-pkg/core/service/business"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	mm "github.com/pipewave-dev/go-pkg/pkg/mux-middleware"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	healthyprovider "github.com/pipewave-dev/go-pkg/provider/healthy-provider"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (delivery.ModuleDelivery, error) {
	ins := &moduleDelivery{
		c:             do.MustInvoke[configprovider.ConfigStore](i),
		mux:           http.NewServeMux(),
		mw:            do.MustInvoke[mm.MiddlewareProvider](i),
		wsDeli:        do.MustInvoke[wsSv.ServerDelivery](i),
		wsService:     do.MustInvoke[wsSv.WsService](i),
		wsOnNewReg:    do.MustInvoke[wsSv.OnNewStuffFn](i),
		wsOnCloseReg:  do.MustInvoke[wsSv.OnCloseStuffFn](i),
		healthy:       do.MustInvoke[healthyprovider.Healthy](i),
		monitoringSvc: do.MustInvoke[business.Monitoring](i),
		metrics:       metrics.New(),

		workerPool:   do.MustInvoke[*workerpool.WorkerPool](i),
		cleanupTask:  do.MustInvoke[fncollector.CleanupTask](i),
		intervalTask: do.MustInvoke[fncollector.IntervalTask](i),
		repo:         do.MustInvoke[repository.AllRepository](i),
	}
	ins.registerHandlers()
	stopFn := ins.runIntervalTasks(time.Second * 600)
	cleanupTask := do.MustInvoke[fncollector.CleanupTask](i)
	cleanupTask.RegTask(stopFn, fncollector.FnPriorityEarlyest) // Stop to prevent push new tasks
	return ins, nil
}

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
	metrics       *metrics.PipewaveMetrics

	workerPool   *workerpool.WorkerPool
	cleanupTask  fncollector.CleanupTask
	intervalTask fncollector.IntervalTask
	repo         repository.AllRepository
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
