package workerpool

import (
	"sync"
	"sync/atomic"
)

type WorkerPoolCfn struct {
	Workers        int
	Buffer         int
	UpperThreshold Threshold // If queue length exceeds this value, trigger action
	LowerThreshold Threshold // If queue length goes below this value, trigger action
	PanicHandler   func(recoverValue any)
}

type Threshold struct {
	Value  int
	Action func()
}

// WorkerPool processes tasks without blocking the event loop.
type WorkerPool struct {
	workers        int
	upperThreshold Threshold
	lowerThreshold Threshold
	panicHandler   func(recoverValue any)
	taskQueue      chan func()
	wg             sync.WaitGroup
	done           chan struct{}
	dropped        atomic.Int64

	// closeMu guards against the race between Submit (RLock) sending on
	// taskQueue and Close (Lock) closing it. Close only closes taskQueue
	// after every in-flight Submit has released the read lock, so Submit
	// can never observe a closed channel mid-send.
	closeMu sync.RWMutex
	closed  bool
}
type WorkerPoolStat struct {
	QueueLength   int   `json:"queue_length"`
	QueueCapacity int   `json:"queue_capacity"`
	DroppedTasks  int64 `json:"dropped_tasks"`
}
