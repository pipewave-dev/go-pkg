package msghub

import "sync/atomic"

// ShutdownSignal is a shared value injected into both mediatorSvc and serverDelivery.
// mediatorSvc calls MarkShuttingDown() before closing connections.
// serverDelivery checks IsShuttingDown() in onCloseRegister to skip the temp-disconnect path.
// This avoids a circular Wire dependency between mediatorSvc and serverDelivery.
type ShutdownSignal struct {
	v atomic.Bool
}

func NewShutdownSignal() *ShutdownSignal { return &ShutdownSignal{} }

func (s *ShutdownSignal) MarkShuttingDown() { s.v.Store(true) }
func (s *ShutdownSignal) IsShuttingDown() bool { return s.v.Load() }
