package pipewave

import (
	"log/slog"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/sdk/adapters"
	"github.com/pipewave-dev/go-pkg/sdk/types"
	"github.com/samber/do/v2"
)

type (
	ConfigStore    = configprovider.ConfigStore
	ModuleDelivery = delivery.ModuleDelivery

	PipewaveConfig struct {
		SlogIns           *slog.Logger
		ConfigStore       ConfigStore
		RepositoryFactory adapters.RepositoryAdapter
		QueueFactory      adapters.QueueAdapter
		PubsubFactory     adapters.PubsubAdapter
	}
)

func NewPipewave(config PipewaveConfig) ModuleDelivery {
	if config.SlogIns == nil {
		config.SlogIns = slog.Default()
	}
	rf := config.RepositoryFactory
	if rf == nil {
		rf = adapters.PostgresRepo
	}
	qf := config.QueueFactory
	if qf == nil {
		qf = adapters.QueueValkey
	}
	pf := config.PubsubFactory
	if pf == nil {
		pf = adapters.PubsubValkey
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
