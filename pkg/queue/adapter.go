package queue

import (
	"context"
	"time"
)

// Adapter is the interface for a simple FIFO message queue used by transports
// such as long-polling to buffer messages for a client.
//
// Implementation note – channel TTL:
// Channels used by long-polling have the format "lp:<UserID>:<InstanceID>".
// When a client disconnects, no consumer will ever drain the channel again
// (InstanceID is ephemeral). Callers SHOULD use SetChannelTTL to set an
// expiration on channels so that unconsumed messages do not accumulate
// indefinitely. Publish does NOT automatically refresh the TTL – the caller
// is responsible for managing channel lifetimes explicitly.
type Adapter interface {
	Publish(ctx context.Context, channel string, message []byte) error
	FetchOne(ctx context.Context, channel string) (message []byte, err error)
	FetchMany(ctx context.Context, channel string, maxItem int) (messages [][]byte, err error)
	BlockFetchOne(ctx context.Context, channel string, timeout time.Duration) ([]byte, error)
	Len(ctx context.Context, channel string) (int64, error)

	// SetChannelTTL sets (or resets) the time-to-live for a channel.
	// If the channel is not consumed within the TTL, all its messages are
	// removed automatically. Callers should invoke this at an appropriate
	// point (e.g. when a polling session starts or is refreshed) rather than
	// relying on automatic renewal inside Publish.
	SetChannelTTL(ctx context.Context, channel string, ttlInSec int64) error

	Healthcheck(ctx context.Context) error
	Close()
}
