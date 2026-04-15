package broadcast

import (
	otelprv "github.com/pipewave-dev/go-pkg/pkg/otel"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
)

type (
	channelType string
	// broadcastDI holds injected dependencies for the broadcast package
	broadcastDI struct {
		c           configprovider.ConfigStore
		pubsub      pubsub.PubsubProvider[channelType, *pubsubMessage]
		otel        otelprv.OtelProvider
		cleanupTask fncollector.CleanupTask
	}
)

func newBroadcastDI(
	c configprovider.ConfigStore,
	pubsubAdapter pubsub.Adapter,
	otel otelprv.OtelProvider,
	cleanupTask fncollector.CleanupTask,
) *broadcastDI {
	return &broadcastDI{
		c:           c,
		pubsub:      pubsub.New[channelType, *pubsubMessage](pubsubAdapter),
		otel:        otel,
		cleanupTask: cleanupTask,
	}
}
