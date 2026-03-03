package pubsub

import (
	"context"
)

type Adapter interface {
	Publish(ctx context.Context, channel string, message []byte) error
	Subscribe(channel string, handler func(message []byte)) (unsubscribe func(), err error)

	Healthcheck() error
	Flush()
}
