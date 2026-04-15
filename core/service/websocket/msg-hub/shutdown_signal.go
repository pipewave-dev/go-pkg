package msghub

import (
	"sync/atomic"

	"github.com/samber/do/v2"
)

func NewShutdownSignalDI(i do.Injector) (*ShutdownSignal, error) {
	return &ShutdownSignal{}, nil
}

// ShutdownSignal is a shared value injected into both mediatorSvc and serverDelivery.
// mediatorSvc calls MarkShuttingDown() before closing connections.
// serverDelivery checks IsShuttingDown() in onCloseRegister to skip the temp-disconnect path.
// This avoids a circular Wire dependency between mediatorSvc and serverDelivery.
type ShutdownSignal struct {
	v atomic.Bool
}

func (s *ShutdownSignal) MarkShuttingDown()    { s.v.Store(true) }
func (s *ShutdownSignal) IsShuttingDown() bool { return s.v.Load() }
