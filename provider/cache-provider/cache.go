package cacheprovider

import (
	"github.com/pipewave-dev/go-pkg/global/constants"
	"github.com/pipewave-dev/go-pkg/pkg/cache"
	valkeyadapter "github.com/pipewave-dev/go-pkg/pkg/cache/adapters/valkey"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	"github.com/samber/do/v2"
	"github.com/samber/lo"
)

func NewDI(i do.Injector) (cache.CacheProvider, error) {
	cfg := do.MustInvoke[configprovider.ConfigStore](i)
	cleanupTask := do.MustInvoke[fncollector.CleanupTask](i)
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

	return cachePrv, nil
}
