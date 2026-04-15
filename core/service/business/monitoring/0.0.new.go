package monitoring

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/core/service/business"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/pkg/cache"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (business.Monitoring, error) {
	allRepo := do.MustInvoke[repository.AllRepository](i)
	obs := do.MustInvoke[observer.Observability](i)
	return &monitoringService{
		activeConnRepo: allRepo.ActiveConnStore(),
		connManager:    do.MustInvoke[wsSv.ConnectionManager](i),
		workerPool:     do.MustInvoke[*workerpool.WorkerPool](i),
		obs:            obs,
		cache:          do.MustInvoke[cache.CacheProvider](i),
	}, nil
}

type monitoringService struct {
	activeConnRepo repository.ActiveConnStore
	connManager    wsSv.ConnectionManager
	workerPool     *workerpool.WorkerPool
	obs            observer.Observability
	cache          cache.CacheProvider
}

// New creates a new Monitoring service instance
func New(
	repo repository.AllRepository,
	connManager wsSv.ConnectionManager,
	workerPool *workerpool.WorkerPool,
	obs observer.Observability,
	cache cache.CacheProvider,
) business.Monitoring {
	return &monitoringService{
		activeConnRepo: repo.ActiveConnStore(),
		connManager:    connManager,
		workerPool:     workerPool,
		obs:            obs,
		cache:          cache,
	}
}
