package pipewave

import (
	"log/slog"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	"github.com/pipewave-dev/go-pkg/core/repository"
	impldynamodb "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb"
	implpostgres "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	pubsubprovider "github.com/pipewave-dev/go-pkg/provider/pubsub"
	queueprovider "github.com/pipewave-dev/go-pkg/provider/queue"
	"github.com/samber/do/v2"
)

type PipewaveConfig struct {
	SlogIns           *slog.Logger
	ConfigStore       ConfigStore
	RepositoryFactory RepositoryDIFactory
	QueueFactory      QueueDIFactory
	PubsubFactory     PubsubDIFactory
}

func NewPipewave(config PipewaveConfig) delivery.ModuleDelivery {
	if config.SlogIns == nil {
		config.SlogIns = slog.Default()
	}
	rf := config.RepositoryFactory
	if rf == nil {
		rf = implpostgres.NewDIPostgresRepo
	}
	qf := config.QueueFactory
	if qf == nil {
		qf = queueprovider.QueueValkeyDI
	}
	pf := config.PubsubFactory
	if pf == nil {
		pf = pubsubprovider.PubsubValkeyDI
	}
	x := do.New(injectionPackage(
		config.ConfigStore,
		config.SlogIns,
		rf,
		qf,
		pf,
	))
	return do.MustInvoke[delivery.ModuleDelivery](x)
}

func ConfigFromYaml(yamlFiles []string) ConfigStore {
	return configprovider.FromYaml(
		yamlFiles,
	)
}

func ConfigFromStruct(configStruct ConfigEnv) ConfigStore {
	return configprovider.FromGoStruct(
		configStruct,
	)
}

type (
	ConfigEnv         = configprovider.EnvType
	ActiveConnectionT = configprovider.ActiveConnectionT
	PingCheckerT      = configprovider.PingCheckerT
	RateLimiterT      = configprovider.RateLimiterT
	WorkerPoolT       = configprovider.WorkerPoolT
	CorsConfigT       = configprovider.CorsConfigT
	OtelT             = configprovider.OtelT
	ValkeyT           = configprovider.ValkeyT
	DynamoConfigT     = configprovider.DynamoConfigT
	DynamoTables      = configprovider.DynamoTables
	PostgresT         = configprovider.PostgresT

	ConfigStore         = configprovider.ConfigStore
	FunctionStore       = *configprovider.Fns
	RepositoryDIFactory = repository.RepositoryDIFactory
	QueueDIFactory      = queueprovider.QueueDIFactory
	PubsubDIFactory     = pubsubprovider.PubsubDIFactory
)

var (
	// Adapters for repositories
	PostgresRepo = implpostgres.NewDIPostgresRepo
	DynamoRepo   = impldynamodb.NewDIDynamoDBRepo

	// Adapters for queues
	QueueValkey = queueprovider.QueueValkeyDI

	// Adapters for pubsub
	PubsubValkey = pubsubprovider.PubsubValkeyDI
)
