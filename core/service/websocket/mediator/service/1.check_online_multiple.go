package mediatorsvc

import (
	"context"

	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func (m *mediatorSvc) CheckOnlineMultiple(ctx context.Context, userIDs []string) (map[string]bool, aerror.AError) {
	counts, aErr := m.activeConnRepo.CountActiveConnectionsBatch(ctx, userIDs)
	if aErr != nil {
		return nil, aErr
	}

	result := make(map[string]bool, len(counts))
	for userID, count := range counts {
		result[userID] = count > 0
	}
	return result, nil
}
