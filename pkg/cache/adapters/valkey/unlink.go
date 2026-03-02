package valkeyadapter

import (
	"context"
	"log/slog"
)

func (vk *valkeyAdapter) Unlink(ctx context.Context, keys ...string) (deleted bool) {
	if vk.keyPrefix != nil {
		for i, key := range keys {
			keys[i] = *vk.keyPrefix + key
		}
	}
	// get value from cache
	c := vk.primClient
	err := c.Do(ctx, c.B().Unlink().Key(keys...).Build()).Error()
	if err != nil {
		slog.ErrorContext(ctx, "valkey.Unlink", slog.Any("err", err))
		return false
	}

	return true
}
