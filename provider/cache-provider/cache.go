package cacheprovider

import (
	"github.com/pipewave-dev/go-pkg/global/constants"
	"github.com/pipewave-dev/go-pkg/pkg/cache"
	valkeyadapter "github.com/pipewave-dev/go-pkg/pkg/cache/adapters/valkey"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	"github.com/samber/lo"
)

// New creates a new cache provider with injected config.
// This replaces the singleton pattern in singleton/cache with dependency injection.
func New(cfg configprovider.ConfigStore, cleanupTask fncollector.CleanupTask) cache.CacheProvider {
	env := cfg.Env()

	cachePrv := cache.New(valkeyadapter.New(&valkeyadapter.ValkeyConfig{
		PrimaryAddress: env.Valkey.PrimaryAddress,
		ReplicaAddress: env.Valkey.ReplicaAddress,
		Password:       env.Valkey.Password,
		DatabaseIndex:  env.Valkey.DatabaseIdx,
		KeyPrefix:      lo.ToPtr(constants.AppNameShort + env.Env),
	}))

	// Register cleanup task
	cleanupTask.RegTask(func() {
		cachePrv.Flush()
	}, fncollector.FnPriorityNormal)

	return cachePrv
}
