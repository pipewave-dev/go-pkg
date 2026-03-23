package app

import (
	"github.com/pipewave-dev/go-pkg/core/delivery"
	"github.com/pipewave-dev/go-pkg/core/repository"
	wirecollection "github.com/pipewave-dev/go-pkg/gen/wire"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	pkgpubsub "github.com/pipewave-dev/go-pkg/pkg/pubsub"
	pkgqueue "github.com/pipewave-dev/go-pkg/pkg/queue"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	pubsubfactory "github.com/pipewave-dev/go-pkg/provider/pubsub"
	"github.com/pipewave-dev/go-pkg/provider/queue"
	"github.com/google/wire"
)

// Make new type for sumary handler interactor
type AppDI struct {
	Delivery delivery.ModuleDelivery
}

func NewAppDI(d delivery.ModuleDelivery) *AppDI {
	return &AppDI{
		Delivery: d,
	}
}

func RepoProvider(
	f repository.RepoFactory,
	c configprovider.ConfigStore,
	obs observer.Observability,
) repository.AllRepository {
	return f(c, obs)
}

func QueueProvider(
	f queue.QueueFactory,
	c configprovider.ConfigStore,
	cleanupTask fncollector.CleanupTask,
) pkgqueue.Adapter {
	return f(c, cleanupTask)
}

func PubsubProvider(
	f pubsubfactory.PubsubFactory,
	c configprovider.ConfigStore,
	cleanupTask fncollector.CleanupTask,
) pkgpubsub.Adapter {
	return f(c, cleanupTask)
}

var IteractorCollection = wire.NewSet(
	wirecollection.DefaultWireSet,
	NewAppDI,
	RepoProvider,
	QueueProvider,
	PubsubProvider,
)

// func InitializeEvent(phrase string) (Event, error) {
//      // woops! NewEventNumber is unused.
//     wire.Build(NewEvent, NewGreeter, NewMessage, NewEventNumber)
//     return Event{}, nil
// }
