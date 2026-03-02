package fncollector

import "sync"

type IntervalTask interface {
	FnCollector
}

func NewIntervalTask() IntervalTask {
	return &stuffsFn{
		fnItems: make([]fnItem, 0),
		mu:      sync.Mutex{},
	}
}
