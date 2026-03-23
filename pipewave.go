package pipewave

import (
	"log/slog"

	"github.com/pipewave-dev/go-pkg/app"
	"github.com/pipewave-dev/go-pkg/core/delivery"
	"github.com/pipewave-dev/go-pkg/core/repository"
	implpostgres "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	pubsubprovider "github.com/pipewave-dev/go-pkg/provider/pubsub"
	queueprovider "github.com/pipewave-dev/go-pkg/provider/queue"
)

type FunctionStore = configprovider.Fns

type PipewaveConfig struct {
	ConfigStore       configprovider.ConfigStore
	RepositoryFactory repository.RepoFactory
	QueueFactory      queueprovider.QueueFactory
	PubsubFactory     pubsubprovider.PubsubFactory
	SlogIns           *slog.Logger
}

func NewPipewave(config PipewaveConfig) delivery.ModuleDelivery {
	if config.SlogIns == nil {
		config.SlogIns = slog.Default()
	}
	rf := config.RepositoryFactory
	if rf == nil {
		rf = implpostgres.NewPostgresRepo
	}
	qf := config.QueueFactory
	if qf == nil {
		qf = queueprovider.QueueValkey
	}
	pf := config.PubsubFactory
	if pf == nil {
		pf = pubsubprovider.PubsubValkey
	}
	x := app.NewPipewave(
		config.ConfigStore,
		config.SlogIns,
		rf,
		qf,
		pf)
	return x.Delivery
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
