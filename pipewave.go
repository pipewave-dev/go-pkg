package pipewave

import (
	"log/slog"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	"github.com/pipewave-dev/go-pkg/export/adapters"
	"github.com/pipewave-dev/go-pkg/export/types"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/samber/do/v2"
)

type (
	ConfigStore    = configprovider.ConfigStore
	ModuleDelivery = delivery.ModuleDelivery

	PipewaveConfig struct {
		SlogIns *slog.Logger
		/* You can use the one of functions
		- ConfigFromYAML
		- ConfigFromStruct
		*/
		ConfigStore ConfigStore
		/* You can use the provided by the package.
		- `github.com/pipewave-dev/go-pkg/export/adapters/repo/dynamodb`
		- `github.com/pipewave-dev/go-pkg/export/adapters/repo/postgresql`
		*/
		RepositoryFactory adapters.RepositoryAdapter
		/* You can use the provided by the package.
		- `github.com/pipewave-dev/go-pkg/export/adapters/queue/valkey`
		*/
		QueueFactory adapters.QueueAdapter
		/* You can use the provided by the package.
		- `github.com/pipewave-dev/go-pkg/export/adapters/pubsub/valkey`
		*/
		PubsubFactory adapters.PubsubAdapter
	}
)

func New(config PipewaveConfig) ModuleDelivery {
	if config.SlogIns == nil {
		config.SlogIns = slog.Default()
	}
	if config.RepositoryFactory == nil || config.QueueFactory == nil || config.PubsubFactory == nil {
		panic("RepositoryFactory, QueueFactory, and PubsubFactory must be provided")
	}

	x := do.New(injectionPackage(
		config.ConfigStore,
		config.SlogIns,
		config.RepositoryFactory,
		config.QueueFactory,
		config.PubsubFactory,
	))
	return do.MustInvoke[delivery.ModuleDelivery](x)
}

func ConfigFromYAML(yamlFiles []string) ConfigStore {
	return configprovider.FromYaml(
		yamlFiles,
	)
}

func ConfigFromStruct(configStruct types.EnvType) ConfigStore {
	return configprovider.FromGoStruct(
		configStruct,
	)
}
