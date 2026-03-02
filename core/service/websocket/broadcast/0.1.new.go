package broadcast

import (
	otelprv "github.com/pipewave-dev/go-pkg/pkg/otel"
	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
)

// broadcastDI holds injected dependencies for the broadcast package
type broadcastDI struct {
	pubsub      pubsub.PubsubProvider[pubsubChannel, *pubsubMessage]
	otel        otelprv.OtelProvider
	cleanupTask fncollector.CleanupTask
}

func newBroadcastDI(
	pubsubAdapter pubsub.Adapter,
	otel otelprv.OtelProvider,
	cleanupTask fncollector.CleanupTask,
) *broadcastDI {
	return &broadcastDI{
		pubsub:      pubsub.New[pubsubChannel, *pubsubMessage](pubsubAdapter),
		otel:        otel,
		cleanupTask: cleanupTask,
	}
}
