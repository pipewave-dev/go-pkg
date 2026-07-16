package cache

import (
	"context"
	"time"
)

type StoreAdapter interface {
	CacheAdapter()

	Set(ctx context.Context, key string, value string, ttl time.Duration) (setable bool)
	Get(ctx context.Context, key string) (val string, found bool)
	// GetDel atomically reads and deletes key in a single round-trip.
	GetDel(ctx context.Context, key string) (val string, found bool)
	Del(ctx context.Context, key string) (deleted bool)
	Incr(ctx context.Context, key string) (int64, bool)
	Decr(ctx context.Context, key string) (int64, bool)

	Flush() error
}
