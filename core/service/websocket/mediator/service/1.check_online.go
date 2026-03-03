package mediatorsvc

import (
	"context"

	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) CheckOnline(ctx context.Context, userID string) (isOnline bool, aErr aerror.AError) {
	count, aErr := m.activeConnRepo.CountActiveConnections(ctx, userID)
	if aErr != nil {
		return false, aErr
	}
	return count > 0, nil
}
