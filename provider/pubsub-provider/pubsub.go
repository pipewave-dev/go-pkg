package pubsubprovider

import (
	"github.com/pipewave-dev/go-pkg/global/constants"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub/adapters/valkey"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
)

// New creates a new pubsub provider with injected config.
// This replaces the singleton pattern in singleton/pubsub with dependency injection.
func New(cfg configprovider.ConfigStore, cleanupTask fncollector.CleanupTask) pubsub.Adapter {
	env := cfg.Env()

	pubsubIns := valkey.New(&valkey.Config{
		ValkeyEndpoint: env.Valkey.PrimaryAddress,
		Password:       env.Valkey.Password,
		DB:             env.Valkey.DatabaseIdx,
		Prefix:         constants.AppNameShort + env.Env,
	})

	// Register cleanup task
	cleanupTask.RegTask(func() {
		pubsubIns.Flush()
	}, fncollector.FnPriorityNormal)

	return pubsubIns
}
