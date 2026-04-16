package adapters

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	impldynamodb "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb"
	implpostgres "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres"
	pubsubprovider "github.com/pipewave-dev/go-pkg/provider/pubsub"
	queueprovider "github.com/pipewave-dev/go-pkg/provider/queue"
)

type (
	RepositoryAdapter = repository.RepositoryAdapter
	QueueAdapter      = queueprovider.QueueAdapter
	PubsubAdapter     = pubsubprovider.PubsubAdapter
)

var (
	// Adapters for repositories
	PostgresRepo RepositoryAdapter = implpostgres.NewDIPostgresRepo
	DynamoRepo   RepositoryAdapter = impldynamodb.NewDIDynamoDBRepo

	// Adapters for queues
	QueueValkey QueueAdapter = queueprovider.QueueValkeyDI

	// Adapters for pubsub
	PubsubValkey PubsubAdapter = pubsubprovider.PubsubValkeyDI
)
