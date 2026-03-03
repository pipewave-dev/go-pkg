package fncollector

import "sync"

type CleanupTask interface {
	FnCollector
}

func NewCleanupTask() CleanupTask {
	return &stuffsFn{
		fnItems: make([]fnItem, 0),
		mu:      sync.Mutex{},
	}
}
