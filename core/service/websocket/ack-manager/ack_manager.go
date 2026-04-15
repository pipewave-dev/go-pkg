package ackmanager

import (
	"sync"
	"time"

	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (*AckManager, error) {
	return &AckManager{
		pending:    make(map[string]chan struct{}),
		remoteAcks: make(map[string]string),
	}, nil
}

// AckManager manages pending message acknowledgments.
type AckManager struct {
	mu         sync.RWMutex
	pending    map[string]chan struct{}
	remoteAcks map[string]string // ackID → sourceContainerID
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

// RegisterRemoteAck registers an ack that originated on a remote container.
// Called on ContainerB when it receives a SendWithAck pubsub message,
// so that when the client ACKs, it can route the signal back to ContainerA.
func (a *AckManager) RegisterRemoteAck(ackID, sourceContainerID string) {
	a.mu.Lock()
	a.remoteAcks[ackID] = sourceContainerID
	a.mu.Unlock()
}

// ResolveRemoteAck looks up and removes a remote ack entry.
// Returns the sourceContainerID and true if found.
func (a *AckManager) ResolveRemoteAck(ackID string) (sourceContainerID string, ok bool) {
	a.mu.Lock()
	sourceContainerID, ok = a.remoteAcks[ackID]
	if ok {
		delete(a.remoteAcks, ackID)
	}
	a.mu.Unlock()
	return
}

// Shutdown cancels all pending ACKs, unblocking any goroutines waiting in WaitForAck.
// Should be called during graceful shutdown before closing connections.
func (a *AckManager) Shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for ackID, ch := range a.pending {
		close(ch)
		delete(a.pending, ackID)
	}

	for ackID := range a.remoteAcks {
		delete(a.remoteAcks, ackID)
	}
}
