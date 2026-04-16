package pipewave

import (
	"log/slog"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	implpostgres "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	pubsubprovider "github.com/pipewave-dev/go-pkg/provider/pubsub"
	queueprovider "github.com/pipewave-dev/go-pkg/provider/queue"
	"github.com/pipewave-dev/go-pkg/sdk/adapters"
	"github.com/pipewave-dev/go-pkg/sdk/types"
	"github.com/samber/do/v2"
)

type (
	ConfigStore    = configprovider.ConfigStore
	PipewaveConfig struct {
		SlogIns           *slog.Logger
		ConfigStore       ConfigStore
		RepositoryFactory adapters.RepositoryAdapter
		QueueFactory      adapters.QueueAdapter
		PubsubFactory     adapters.PubsubAdapter
	}
)

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

func ConfigFromStruct(configStruct types.EnvType) ConfigStore {
	return configprovider.FromGoStruct(
		configStruct,
	)
}
