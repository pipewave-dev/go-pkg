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

func (p *WorkerPool) Submit(task func()) {
	p.taskQueue <- task
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

// Close shuts down the pool and waits for all workers to finish.
func (p *WorkerPool) Close() {
	close(p.done)
	close(p.taskQueue)
	p.wg.Wait()
}

func (p *WorkerPool) Stat() WorkerPoolStat {
	return WorkerPoolStat{
		QueueLength:   len(p.taskQueue),
		QueueCapacity: cap(p.taskQueue),
	}
}
