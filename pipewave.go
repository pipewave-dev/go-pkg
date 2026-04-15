package pipewave

import (
	"log/slog"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	"github.com/pipewave-dev/go-pkg/core/repository"
	implpostgres "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	pubsubprovider "github.com/pipewave-dev/go-pkg/provider/pubsub"
	queueprovider "github.com/pipewave-dev/go-pkg/provider/queue"
	"github.com/samber/do/v2"
)

type FunctionStore = configprovider.Fns

type PipewaveConfigDI struct {
	ConfigStore       configprovider.ConfigStore
	RepositoryFactory repository.RepositoryDIFactory
	QueueFactory      queueprovider.QueueDIFactory
	PubsubFactory     pubsubprovider.PubsubDIFactory
	SlogIns           *slog.Logger
}

func NewPipewave(config PipewaveConfigDI) delivery.ModuleDelivery {
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

func ConfigFromYaml(yamlFiles []string, fnStore FunctionStore) configprovider.ConfigStore {
	return configprovider.FromYaml(
		yamlFiles,
	)
}

type ConfigEnv = configprovider.EnvType

func ConfigFromStruct(configStruct ConfigEnv, fnStore FunctionStore) configprovider.ConfigStore {
	return configprovider.FromGoStruct(
		configStruct,
	)
}
