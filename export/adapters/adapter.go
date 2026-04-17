package adapters

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	pubsubprovider "github.com/pipewave-dev/go-pkg/provider/pubsub"
	queueprovider "github.com/pipewave-dev/go-pkg/provider/queue"
)

type (
	RepositoryAdapter = repository.RepositoryAdapter
	QueueAdapter      = queueprovider.QueueAdapter
	PubsubAdapter     = pubsubprovider.PubsubAdapter
)
