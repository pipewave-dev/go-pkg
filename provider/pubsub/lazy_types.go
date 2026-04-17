package pubsub

import (
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	"github.com/samber/do/v2"
)

type (
	PubsubAdapter = func(i do.Injector) (pubsub.Adapter, error)
)
