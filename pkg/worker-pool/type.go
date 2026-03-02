package workerpool

import "sync"

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
}
type WorkerPoolStat struct {
	QueueLength   int `json:"queue_length"`
	QueueCapacity int `json:"queue_capacity"`
}
