package workerpool

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestPool(workers, buffer int) *WorkerPool {
	pool := New(&WorkerPoolCfn{
		Workers: workers,
		Buffer:  buffer,
		UpperThreshold: Threshold{
			Value:  1 << 30,
			Action: func() {},
		},
		LowerThreshold: Threshold{
			Value:  -1,
			Action: func() {},
		},
	})
	pool.Start()
	return pool
}

func TestSubmitDoesNotBlockWhenQueueFull(t *testing.T) {
	pool := newTestPool(1, 1)
	defer pool.Close()

	block := make(chan struct{})
	defer close(block)
	started := make(chan struct{})

	// Occupy the single worker and wait until it has actually picked up
	// the task, so the next submits deterministically land in/behind the
	// buffer instead of racing the worker for the handoff.
	if !pool.Submit(func() { close(started); <-block }) {
		t.Fatal("expected first submit to succeed")
	}
	<-started

	// Buffer capacity is 1: this fills it since the worker is busy.
	if !pool.Submit(func() { <-block }) {
		t.Fatal("expected second submit to fill the buffer")
	}

	// Worker busy + buffer full: Submit must return immediately with
	// false instead of blocking the caller.
	done := make(chan bool, 1)
	go func() {
		done <- pool.Submit(func() {})
	}()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("expected Submit to be dropped, not accepted, when queue is full")
		}
	case <-time.After(time.Second):
		t.Fatal("Submit blocked instead of returning immediately when queue was full")
	}

	if got := pool.Stat().DroppedTasks; got != 1 {
		t.Fatalf("expected 1 dropped task, got %d", got)
	}
}

func TestSubmitAfterCloseReturnsFalse(t *testing.T) {
	pool := newTestPool(2, 4)
	pool.Close()

	if pool.Submit(func() {}) {
		t.Fatal("expected Submit to be rejected after Close")
	}
	if got := pool.Stat().DroppedTasks; got != 1 {
		t.Fatalf("expected 1 dropped task, got %d", got)
	}
}

// TestConcurrentSubmitDuringCloseNeverPanics reproduces the shutdown panic
// from ai-feedback/03: many goroutines calling Submit concurrently with
// Close must never panic with "send on closed channel".
func TestConcurrentSubmitDuringCloseNeverPanics(t *testing.T) {
	pool := newTestPool(2, 8)

	var wg sync.WaitGroup
	var accepted, rejected atomic.Int64
	stop := make(chan struct{})

	for range 50 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if pool.Submit(func() {}) {
					accepted.Add(1)
				} else {
					rejected.Add(1)
				}
			}
		})
	}

	time.Sleep(20 * time.Millisecond)
	pool.Close() // must not panic despite concurrent Submit callers above
	close(stop)
	wg.Wait()

	if accepted.Load() == 0 {
		t.Fatal("expected at least some submits to succeed before close")
	}
}
