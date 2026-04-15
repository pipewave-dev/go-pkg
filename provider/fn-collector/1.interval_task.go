package fncollector

import (
	"sync"

	"github.com/samber/do/v2"
)

func NewDIIntervalTask(i do.Injector) (IntervalTask, error) {
	return &stuffsFn{
		fnItems: make([]fnItem, 0),
		mu:      sync.Mutex{},
	}, nil
}

type IntervalTask interface {
	FnCollector
}
