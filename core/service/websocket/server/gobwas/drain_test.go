package gobwas

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockDrainableConn simulates a connection that records the order in which payloads are sent.
type mockDrainableConn struct {
	drainMu sync.RWMutex
	sent    []string
	mu      sync.Mutex
}

func (c *mockDrainableConn) send(msg string) {
	c.drainMu.RLock()
	defer c.drainMu.RUnlock()
	c.mu.Lock()
	c.sent = append(c.sent, msg)
	c.mu.Unlock()
}

func (c *mockDrainableConn) sendDirect(msg string) {
	// No drainMu — caller holds WLock
	c.mu.Lock()
	c.sent = append(c.sent, msg)
	c.mu.Unlock()
}

func (c *mockDrainableConn) beginDrain() { c.drainMu.Lock() }
func (c *mockDrainableConn) endDrain()   { c.drainMu.Unlock() }

// TestDrainOrdering verifies that pending messages sent via sendDirect
// during a drain phase always appear before messages sent via send()
// from concurrent goroutines.
func TestDrainOrdering(t *testing.T) {
	const iterations = 100

	for i := range iterations {
		conn := &mockDrainableConn{}
		conn.beginDrain()

		var wg sync.WaitGroup
		var started atomic.Bool

		// Simulate 5 concurrent senders that fire as soon as drain begins.
		for j := range 5 {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				// Spin until drain has actually started so the race is real.
				for !started.Load() {
					time.Sleep(time.Microsecond)
				}
				conn.send("new")
			}(j)
		}

		started.Store(true)

		// Give goroutines a chance to reach drainMu.RLock and block.
		time.Sleep(time.Millisecond)

		// Drain pending messages directly (WLock held).
		conn.sendDirect("pending-1")
		conn.sendDirect("pending-2")

		conn.endDrain()
		wg.Wait()

		// Verify: first two messages must be the pending ones.
		conn.mu.Lock()
		got := conn.sent
		conn.mu.Unlock()

		if len(got) < 2 {
			t.Fatalf("iter %d: expected at least 2 messages, got %d", i, len(got))
		}
		if got[0] != "pending-1" || got[1] != "pending-2" {
			t.Errorf("iter %d: expected [pending-1 pending-2 ...], got %v", i, got)
		}
	}
}
