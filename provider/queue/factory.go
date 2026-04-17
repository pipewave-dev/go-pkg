package queue

import (
	"github.com/pipewave-dev/go-pkg/pkg/queue"
	"github.com/samber/do/v2"
)

type (
	QueueAdapter = func(i do.Injector) (queue.Adapter, error)
)
