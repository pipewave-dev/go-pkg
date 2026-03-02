package wirecollection

import (
	"github.com/google/wire"
	module "github.com/pipewave-dev/go-pkg/core/delivery/module"
	monitoring "github.com/pipewave-dev/go-pkg/core/service/business/monitoring"
	broadcast_msg_handler "github.com/pipewave-dev/go-pkg/core/service/websocket/broadcast-msg-handler"
	client_msg_handler "github.com/pipewave-dev/go-pkg/core/service/websocket/client-msg-handler"
	connection_manager "github.com/pipewave-dev/go-pkg/core/service/websocket/connection-manager"
	exchange_token "github.com/pipewave-dev/go-pkg/core/service/websocket/exchange-token"
	delivery "github.com/pipewave-dev/go-pkg/core/service/websocket/mediator/delivery"
	service "github.com/pipewave-dev/go-pkg/core/service/websocket/mediator/service"
	rate_limiter "github.com/pipewave-dev/go-pkg/core/service/websocket/rate-limiter"
	gobwas "github.com/pipewave-dev/go-pkg/core/service/websocket/server/gobwas"
	ws_event_trigger "github.com/pipewave-dev/go-pkg/core/service/websocket/ws-event-trigger"
	cache_provider "github.com/pipewave-dev/go-pkg/provider/cache-provider"
	fn_collector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	healthy_provider "github.com/pipewave-dev/go-pkg/provider/healthy-provider"
	mux_middleware "github.com/pipewave-dev/go-pkg/provider/mux-middleware"
	observer_provider "github.com/pipewave-dev/go-pkg/provider/observer-provider"
	otel_provider "github.com/pipewave-dev/go-pkg/provider/otel-provider"
	pubsub_provider "github.com/pipewave-dev/go-pkg/provider/pubsub-provider"
	time_provider "github.com/pipewave-dev/go-pkg/provider/time-provider"
	validation_provider "github.com/pipewave-dev/go-pkg/provider/validation-provider"
	worker_pool_provider "github.com/pipewave-dev/go-pkg/provider/worker-pool-provider"
)

var DefaultWireSet = wire.NewSet(
	// Default WireSet collects all WireSets without a specific name
	module.WireSet,
	monitoring.WireSet,
	broadcast_msg_handler.WireSet,
	client_msg_handler.WireSet,
	connection_manager.WireSet,
	exchange_token.WireSet,
	delivery.WireSet,
	service.WireSet,
	rate_limiter.WireSet,
	gobwas.WireSet,
	ws_event_trigger.WireSet,
	cache_provider.WireSet,
	fn_collector.WireSet,
	healthy_provider.WireSet,
	mux_middleware.WireSet,
	observer_provider.WireSet,
	otel_provider.WireSet,
	pubsub_provider.WireSet,
	time_provider.WireSet,
	validation_provider.WireSet,
	worker_pool_provider.WireSet,
)
