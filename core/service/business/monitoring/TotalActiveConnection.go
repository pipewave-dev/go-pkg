package monitoring

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	cacheprovider "github.com/pipewave-dev/go-pkg/provider/cache-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnTotalActiveConnection = "monitoringService.TotalActiveConnection"

func (m *monitoringService) TotalActiveConnection(ctx context.Context) (total int, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = m.obs.StartOperation(ctx, fnTotalActiveConnection)
	defer op.Finish(aErr)

	total64, aErr := m.activeConnRepo.CountTotalActiveConnections(ctx)
	if aErr != nil {
		return 0, aErr
	}

	cacheprovider.CacheThis(ctx, m.cache, 2*time.Minute,
		"total_active_connection",
		func(ctx context.Context) (int, aerror.AError) {
			total64, aErr := m.activeConnRepo.CountTotalActiveConnections(ctx)
			if aErr != nil {
				return 0, aErr
			}
			return int(total64), nil
		})

	return int(total64), nil
}
