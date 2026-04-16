package pipewave

import (
	"log/slog"

	pubsubprovider "github.com/pipewave-dev/go-pkg/provider/pubsub"
	queueprovider "github.com/pipewave-dev/go-pkg/provider/queue"
	"github.com/samber/do/v2"

	moduledelivery "github.com/pipewave-dev/go-pkg/core/delivery/module"
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/core/service/business/monitoring"
	ackmanager "github.com/pipewave-dev/go-pkg/core/service/websocket/ack-manager"
	broadcastmsghandler "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast-msg-handler"
	clientmsghandler "github.com/pipewave-dev/go-pkg/core/service/websocket/client-msg-handler"
	connectionmanager "github.com/pipewave-dev/go-pkg/core/service/websocket/connection-manager"
	exchangetoken "github.com/pipewave-dev/go-pkg/core/service/websocket/exchange-token"
	wsDelivery "github.com/pipewave-dev/go-pkg/core/service/websocket/mediator/delivery"
	mediatorsvc "github.com/pipewave-dev/go-pkg/core/service/websocket/mediator/service"
	msghub "github.com/pipewave-dev/go-pkg/core/service/websocket/msg-hub"
	ratelimiter "github.com/pipewave-dev/go-pkg/core/service/websocket/rate-limiter"
	wseventtrigger "github.com/pipewave-dev/go-pkg/core/service/websocket/ws-event-trigger"
	cacheprovider "github.com/pipewave-dev/go-pkg/provider/cache-provider"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	healthyprovider "github.com/pipewave-dev/go-pkg/provider/healthy-provider"
	muxmiddleware "github.com/pipewave-dev/go-pkg/provider/mux-middleware"
	observerprovider "github.com/pipewave-dev/go-pkg/provider/observer-provider"
	otelprovider "github.com/pipewave-dev/go-pkg/provider/otel-provider"
	workerpoolprovider "github.com/pipewave-dev/go-pkg/provider/worker-pool-provider"

	_ "github.com/pipewave-dev/go-pkg/shared/aerror"
)

func injectionPackage(
	configStore configprovider.ConfigStore,
	logger *slog.Logger,
	repositoryFactory repository.RepositoryAdapter,
	queueFactory queueprovider.QueueAdapter,
	pubsubFactory pubsubprovider.PubsubAdapter,
) func(do.Injector) {
	return do.Package(
		// Inject from parameters
		do.Eager(logger),
		do.Eager(configStore),
		do.Lazy(repositoryFactory),
		do.Lazy(queueFactory),
		do.Lazy(pubsubFactory),
		// Inject from functions
		do.Lazy(connectionmanager.NewDI),
		do.Lazy(moduledelivery.NewDI),
		do.Lazy(monitoring.NewDI),
		do.Lazy(ackmanager.NewDI),
		do.Lazy(broadcastmsghandler.NewDI),
		do.Lazy(clientmsghandler.NewDI),
		do.Lazy(exchangetoken.NewDI),
		do.Lazy(wsDelivery.NewDI),
		do.Lazy(mediatorsvc.NewDI),
		do.Lazy(msghub.NewDI),
		do.Lazy(msghub.NewShutdownSignalDI),
		do.Lazy(ratelimiter.NewDI),
		do.Lazy(wseventtrigger.NewDIOnCloseStuff),
		do.Lazy(wseventtrigger.NewDIOnNewStuff),
		do.Lazy(cacheprovider.NewDI),
		do.Lazy(fncollector.NewDICleanupTask),
		do.Lazy(fncollector.NewDIIntervalTask),
		do.Lazy(healthyprovider.NewDI),
		do.Lazy(muxmiddleware.NewDI),
		do.Lazy(observerprovider.NewDI),
		do.Lazy(otelprovider.NewDI),
		do.Lazy(workerpoolprovider.NewDI),
	)
}
