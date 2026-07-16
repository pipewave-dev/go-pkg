package workerpool

import "time"

func New(cfg *WorkerPoolCfn) *WorkerPool {
	pool := &WorkerPool{
		workers:        cfg.Workers,
		upperThreshold: cfg.UpperThreshold,
		lowerThreshold: cfg.LowerThreshold,
		taskQueue:      make(chan func(), cfg.Buffer),
		done:           make(chan struct{}),
		panicHandler:   cfg.PanicHandler,
	}

	return pool
}

func (p *WorkerPool) worker() {
	defer p.wg.Done()
	for task := range p.taskQueue {
		func() {
			defer func() {
				if r := recover(); r != nil && p.panicHandler != nil {
					p.panicHandler(r)
				}
			}()
			task()
		}()
	}
}

// Submit enqueues task without blocking the caller. It returns false — and
// counts the task as dropped (see Stat) — if the pool is closed or the queue
// is full, instead of blocking the caller (e.g. the netpoll dispatch
// goroutine) or panicking on a closed channel.
func (p *WorkerPool) Submit(task func()) bool {
	p.closeMu.RLock()
	defer p.closeMu.RUnlock()

	if p.closed {
		p.dropped.Add(1)
		return false
	}

	select {
	case p.taskQueue <- task:
		return true
	default:
		p.dropped.Add(1)
		return false
	}
}

// Start launches the worker goroutines.
func (p *WorkerPool) Start() {
	p.wg.Add(p.workers)
	for range p.workers {
		go p.worker()
	}
	// Monitoring queue length for thresholds can be added here if needed
	go p.monitorQueue()
}

func (p *WorkerPool) monitorQueue() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var wasAboveUpper bool
	var wasBelowLower bool

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			queueLen := len(p.taskQueue)

			// Check for transition from low to high threshold
			isAboveUpper := queueLen > p.upperThreshold.Value
			if p.upperThreshold.Action != nil && isAboveUpper && !wasAboveUpper {
				p.upperThreshold.Action()
			}
			wasAboveUpper = isAboveUpper

			// Check for transition from high to low threshold
			isBelowLower := queueLen < p.lowerThreshold.Value
			if p.lowerThreshold.Action != nil && isBelowLower && !wasBelowLower {
				p.lowerThreshold.Action()
			}
			wasBelowLower = isBelowLower
		}
	}
}

// Close shuts down the pool and waits for all already-queued tasks to
// finish. It blocks until every in-flight Submit call has returned before
// closing taskQueue, so Submit can never send on a closed channel.
func (p *WorkerPool) Close() {
	close(p.done)

	p.closeMu.Lock()
	p.closed = true
	close(p.taskQueue)
	p.closeMu.Unlock()

	p.wg.Wait()
}

func (p *WorkerPool) Stat() WorkerPoolStat {
	return WorkerPoolStat{
		QueueLength:   len(p.taskQueue),
		QueueCapacity: cap(p.taskQueue),
		DroppedTasks:  p.dropped.Load(),
	}
}
