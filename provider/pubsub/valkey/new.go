package pubsub

import (
	"github.com/pipewave-dev/go-pkg/global/constants"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub/adapters/valkey"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	"github.com/samber/do/v2"
)

func PubsubValkeyDI(i do.Injector) (pubsub.Adapter, error) {
	c := do.MustInvoke[configprovider.ConfigStore](i)
	cleanupTask := do.MustInvoke[fncollector.CleanupTask](i)

	return pubsubValkey(c, cleanupTask), nil
}

func pubsubValkey(c configprovider.ConfigStore, cleanupTask fncollector.CleanupTask) pubsub.Adapter {
	env := c.Env()
	ins := valkey.New(&valkey.Config{
		ValkeyEndpoint: env.Valkey.PrimaryAddress,
		Password:       env.Valkey.Password,
		DB:             env.Valkey.DatabaseIdx,
		Prefix:         constants.AppNameShort + env.Info.Env,
	})

	// Register cleanup task
	cleanupTask.RegTask(func() {
		ins.Flush()
	}, fncollector.FnPriorityNormal)

	return ins
}
