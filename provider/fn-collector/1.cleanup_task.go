package fncollector

import (
	"sync"

	"github.com/samber/do/v2"
)

func NewDICleanupTask(i do.Injector) (CleanupTask, error) {
	return &stuffsFn{
		fnItems: make([]fnItem, 0),
		mu:      sync.Mutex{},
	}, nil
}

type CleanupTask interface {
	FnCollector
}
