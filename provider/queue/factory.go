package queue

import (
	"github.com/pipewave-dev/go-pkg/global/constants"
	"github.com/pipewave-dev/go-pkg/pkg/queue"
	"github.com/pipewave-dev/go-pkg/pkg/queue/adapters/valkey"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	"github.com/samber/do/v2"
)

type (
	QueueAdapter = func(i do.Injector) (queue.Adapter, error)
)

func QueueValkeyDI(i do.Injector) (queue.Adapter, error) {
	c := do.MustInvoke[configprovider.ConfigStore](i)
	cleanupTask := do.MustInvoke[fncollector.CleanupTask](i)

	return QueueValkey(c, cleanupTask), nil
}

func QueueValkey(c configprovider.ConfigStore, cleanupTask fncollector.CleanupTask) queue.Adapter {
	env := c.Env()
	ins := valkey.New(&valkey.Config{
		ValkeyEndpoint: env.Valkey.PrimaryAddress,
		Password:       env.Valkey.Password,
		DB:             env.Valkey.DatabaseIdx,
		Prefix:         constants.AppNameShort + env.Info.Env,
	})

	// Register cleanup task
	cleanupTask.RegTask(func() {
		ins.Close()
	}, fncollector.FnPriorityNormal)

	return ins
}
