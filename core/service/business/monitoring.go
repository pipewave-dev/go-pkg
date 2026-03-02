package business

import (
	"context"
	"fmt"

	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

type SumaryActiveConnection struct {
	AnonymosConnection int
	UserConnection     int
	TotalUser          int
}

func (s SumaryActiveConnection) String() string {
	return fmt.Sprintf("AnonymosConnection: %d, UserConnection: %d, TotalUser: %d", s.AnonymosConnection, s.UserConnection, s.TotalUser)
}

type WorkerPoolSummary struct {
	Length   int
	Capacity int
}

func (s WorkerPoolSummary) String() string {
	return fmt.Sprintf("Length: %d, Capacity: %d", s.Length, s.Capacity)
}

type Monitoring interface {
	// Get connection inside this container (not include other container when using loadbalancer)
	InsideActiveConnection(ctx context.Context) (*SumaryActiveConnection, aerror.AError)

	// Show total active connection across all container (include other container when using loadbalancer)
	TotalActiveConnection(ctx context.Context) (int, aerror.AError)

	WorkerPoolStats(ctx context.Context) (WorkerPoolSummary, aerror.AError)
}
