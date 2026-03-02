package cache

import (
	"context"
	"time"
)

type StoreAdapter interface {
	CacheAdapter()

	Set(ctx context.Context, key string, value string, ttl time.Duration) (setable bool)
	Get(ctx context.Context, key string) (val string, found bool)
	Del(ctx context.Context, key string) (deleted bool)
	Keys(ctx context.Context, pattern string) []string
	Unlink(ctx context.Context, keys ...string) bool
	Incr(ctx context.Context, key string) bool
	Decr(ctx context.Context, key string) bool

	Flush() error
}
