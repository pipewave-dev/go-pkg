package valkeyadapter

import (
	"context"
	"log/slog"
)

func (vk *valkeyAdapter) Del(ctx context.Context, key string) (deleted bool) {
	if vk.keyPrefix != nil {
		key = *vk.keyPrefix + key
	}
	// get value from cache
	c := vk.primClient
	err := c.Do(ctx, c.B().Del().Key(key).Build()).Error()
	if err != nil {
		slog.ErrorContext(ctx, "valkey.Del", slog.Any("err", err))
		return false
	}

	return true
}
