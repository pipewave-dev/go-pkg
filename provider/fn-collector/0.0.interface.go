package fncollector

import (
	"sort"
	"sync"
)

type Priority int

const (
	FnPriorityEarlyest Priority = -9999
	FnPriorityEarler   Priority = -500
	FnPriorityEarly    Priority = -100
	FnPriorityNormal   Priority = 0
	FnPriorityLate     Priority = 100
	FnPriorityLater    Priority = 500
	FnPriorityLatest   Priority = 9999
)

type FnCollector interface {
	RegTask(doStuff func(), priority Priority)
	Get() []func()
}

type fnItem struct {
	priority int
	fn       func()
}

type stuffsFn struct {
	fnItems []fnItem
	mu      sync.Mutex
}

func (s *stuffsFn) RegTask(doStuff func(), priority Priority) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.fnItems = append(s.fnItems, fnItem{
		priority: int(priority),
		fn:       doStuff,
	})
}

func (s *stuffsFn) Get() []func() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Sort by priority, smallest first
	sort.Slice(s.fnItems, func(i, j int) bool {
		return s.fnItems[i].priority < s.fnItems[j].priority
	})

	fns := make([]func(), len(s.fnItems))
	for i, s := range s.fnItems {
		fns[i] = s.fn
	}
	return fns
}
