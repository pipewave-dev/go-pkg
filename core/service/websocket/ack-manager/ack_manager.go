package ackmanager

import (
	"sync"
	"time"

	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

// AckManager manages pending message acknowledgments.
type AckManager struct {
	mu      sync.RWMutex
	pending map[string]chan struct{}
}

// New creates a new AckManager.
func New() *AckManager {
	return &AckManager{
		pending: make(map[string]chan struct{}),
	}
}

// CreateAck creates a new ack ID and returns it with a channel that will be closed when ACK is received.
func (a *AckManager) CreateAck() (ackID string, ch chan struct{}) {
	ackID = "ack_" + fn.NewNanoID(18)
	ch = make(chan struct{})

	a.mu.Lock()
	a.pending[ackID] = ch
	a.mu.Unlock()

	return ackID, ch
}

// ResolveAck resolves a pending ACK. Returns true if the ackID was found and resolved.
func (a *AckManager) ResolveAck(ackID string) bool {
	a.mu.Lock()
	ch, ok := a.pending[ackID]
	if ok {
		delete(a.pending, ackID)
	}
	a.mu.Unlock()

	if ok {
		close(ch)
		return true
	}
	return false
}

// WaitForAck waits for an ACK with a timeout. Returns true if ACK was received.
func (a *AckManager) WaitForAck(ackID string, ch chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ch:
		return true
	case <-timer.C:
		// Timeout — clean up
		a.mu.Lock()
		delete(a.pending, ackID)
		a.mu.Unlock()
		return false
	}
}
