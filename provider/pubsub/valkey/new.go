package pubsub

import (
	"log/slog"
	"os"
	"syscall"

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
		CallbackFn:     restartOnDeadSubscription,
	})

	// Register cleanup task
	cleanupTask.RegTask(func() {
		ins.Flush()
	}, fncollector.FnPriorityNormal)

	return ins
}

// restartOnDeadSubscription is invoked when the Valkey pub/sub subscription could not be
// re-established after retrying. Since a dead subscription silently stops delivering
// broadcast messages, it sends SIGTERM to itself so the process supervisor restarts it
// with a fresh connection rather than keep serving connections that will never see new
// broadcast messages.
func restartOnDeadSubscription() {
	slog.Error("Valkey pub/sub subscription permanently dead, sending SIGTERM to restart process")
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		slog.Error("Failed to send SIGTERM after dead pub/sub subscription", slog.Any("err", err))
	}
}
