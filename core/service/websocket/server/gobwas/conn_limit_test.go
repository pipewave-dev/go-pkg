package gobwas

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// PrintStats is a diagnostic path, so it must never be the thing that takes the
// process down — including before the worker pool is wired up.
func TestPrintStats_SurvivesNilWorkerPool(t *testing.T) {
	s := &NetpollServer{stats: &serverStats{StartTime: time.Now()}}
	s.connections.Store(3)

	s.PrintStats() // must not panic
}

func TestReserveSlot_UncappedAlwaysSucceeds(t *testing.T) {
	s := &NetpollServer{}

	for i := int64(1); i <= 100; i++ {
		got, ok := s.reserveSlot(0)
		if !ok {
			t.Fatalf("reserveSlot(0) refused at %d, want always allowed", i)
		}
		if got != i {
			t.Errorf("reserveSlot(0) observed = %d, want %d", got, i)
		}
	}
}

func TestReserveSlot_RefusesPastLimit(t *testing.T) {
	const limit = 3
	s := &NetpollServer{}

	for i := 1; i <= limit; i++ {
		if _, ok := s.reserveSlot(limit); !ok {
			t.Fatalf("reserveSlot refused slot %d, want allowed up to %d", i, limit)
		}
	}

	observed, ok := s.reserveSlot(limit)
	if ok {
		t.Error("reserveSlot allowed a connection past the limit")
	}
	if observed != limit {
		t.Errorf("observed = %d, want %d", observed, limit)
	}
	// A refused claim must not consume a slot.
	if got := s.connections.Load(); got != limit {
		t.Errorf("connections = %d after refusal, want %d", got, limit)
	}
}

// The cap must hold exactly under concurrent accepts.
//
// The contention has to be aimed at the boundary to be meaningful: every
// goroutine makes exactly one attempt, all released together, with the pool
// pre-filled to one slot below the limit. That way every racer evaluates the
// limit check against the same critical value, which is precisely the
// interleaving a Load-then-Add implementation gets wrong (it overshoots to
// roughly the number of racers). Many attempts per goroutine would instead
// spend most iterations far from the boundary and let the bug slip through.
func TestReserveSlot_ExactUnderConcurrency(t *testing.T) {
	const (
		limit  = 64
		racers = 128
		rounds = 200
	)

	for round := range rounds {
		s := &NetpollServer{}
		s.connections.Store(limit - 1) // one slot left; all racers contend for it

		var granted atomic.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})

		for range racers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start // release everyone at once
				if _, ok := s.reserveSlot(limit); ok {
					granted.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()

		if got := granted.Load(); got != 1 {
			t.Fatalf("round %d: granted = %d, want exactly 1 (the single free slot)", round, got)
		}
		if got := s.connections.Load(); got != limit {
			t.Fatalf("round %d: connections = %d, want exactly %d", round, got, limit)
		}
	}
}

// Releasing a slot must make room for a new connection, so a pod at capacity
// recovers as clients disconnect rather than staying wedged.
func TestReserveSlot_ReleaseFreesCapacity(t *testing.T) {
	const limit = 2
	s := &NetpollServer{}

	for range limit {
		if _, ok := s.reserveSlot(limit); !ok {
			t.Fatal("reserveSlot refused below the limit")
		}
	}
	if _, ok := s.reserveSlot(limit); ok {
		t.Fatal("reserveSlot allowed a connection past the limit")
	}

	s.connections.Add(-1) // simulate a disconnect

	if _, ok := s.reserveSlot(limit); !ok {
		t.Error("reserveSlot refused after a slot was freed, want allowed")
	}
}
