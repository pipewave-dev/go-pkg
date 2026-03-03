package dostuffs

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type stuffsFn struct {
	priority int
	fn       func()
}
type DoStuffs struct {
	stuffsFn   []stuffsFn
	mu         sync.Mutex
	inprogress atomic.Bool
}

func (do *DoStuffs) Do() {
	if do.inprogress.Load() {
		return
	}
	do.inprogress.Store(true)
	defer do.inprogress.Store(false)
	// sort and for loop
	sort.Slice(do.stuffsFn, func(i, j int) bool {
		return do.stuffsFn[i].priority < do.stuffsFn[j].priority
	})
	for _, f := range do.stuffsFn {
		f.fn()
	}
}

func (do *DoStuffs) DoEvery(d time.Duration) (stopFunc func()) {
	ticker := time.NewTicker(d)
	go func() {
		for range ticker.C {
			do.Do()
		}
	}()
	return func() {
		ticker.Stop()
	}
}

func (do *DoStuffs) RegTask(doStuff func(), priority ...int) {
	var p int
	if len(priority) > 0 {
		p = priority[0]
	}
	do.mu.Lock()
	defer do.mu.Unlock()
	do.stuffsFn = append(do.stuffsFn, stuffsFn{
		priority: p,
		fn:       doStuff,
	})
}

var (
	// Clean up function
	CleanTasks = DoStuffs{
		stuffsFn: make([]stuffsFn, 0),
	}

	// Debug function
	DebugFn = DoStuffs{
		stuffsFn: make([]stuffsFn, 0),
	}

	// Interval function
	IntervalFn = DoStuffs{
		stuffsFn: make([]stuffsFn, 0),
	}
)
