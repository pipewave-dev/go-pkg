package app

import (
	"github.com/google/wire"
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

func ActiveConnStoreProvider(allRepository repository.AllRepository) repository.ActiveConnStore {
	return allRepository.ActiveConnStore()
}

func UserRepoProvider(allRepository repository.AllRepository) repository.User {
	return allRepository.User()
}

func PendingMessageRepoProvider(allRepository repository.AllRepository) repository.PendingMessageRepo {
	return allRepository.PendingMessage()
}

var IteractorCollection = wire.NewSet(
	wirecollection.DefaultWireSet,
	NewAppDI,
	RepoProvider,
	ActiveConnStoreProvider,
	UserRepoProvider,
	PendingMessageRepoProvider,
	QueueProvider,
	PubsubProvider,
)

// func InitializeEvent(phrase string) (Event, error) {
//      // woops! NewEventNumber is unused.
//     wire.Build(NewEvent, NewGreeter, NewMessage, NewEventNumber)
//     return Event{}, nil
// }
