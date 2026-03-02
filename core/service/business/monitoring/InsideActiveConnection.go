package monitoring

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/service/business"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

const fnInsideActiveConnection = "monitoringService.InsideActiveConnection"

func (m *monitoringService) InsideActiveConnection(ctx context.Context) (result *business.SumaryActiveConnection, aErr aerror.AError) {
	var op observer.Operation
	ctx, op = m.obs.StartOperation(ctx, fnInsideActiveConnection)
	defer op.Finish(aErr)

	anonymousConns := m.connManager.GetAllAnonymousConn()
	allConns := m.connManager.GetAllConnections()

	uniqueUsers := make(map[string]struct{})
	for _, conn := range allConns {
		uniqueUsers[conn.Auth().UserID] = struct{}{}
	}

	return &business.SumaryActiveConnection{
		AnonymosConnection: len(anonymousConns),
		UserConnection:     len(allConns) - len(anonymousConns),
		TotalUser:          len(uniqueUsers),
	}, nil
}
