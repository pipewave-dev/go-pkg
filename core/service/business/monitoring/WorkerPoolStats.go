package monitoring

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/service/business"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnWorkerPoolStats = "monitoringService.WorkerPoolStats"

func (m *monitoringService) WorkerPoolStats(ctx context.Context) (result business.WorkerPoolSummary, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = m.obs.StartOperation(ctx, fnWorkerPoolStats)
	defer op.Finish(aErr)

	stat := m.workerPool.Stat()

	return business.WorkerPoolSummary{
		Length:   stat.QueueLength,
		Capacity: stat.QueueCapacity,
	}, nil
}
