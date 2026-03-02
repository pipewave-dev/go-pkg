package valkeyadapter

import (
	"context"
	"log/slog"
	"time"
)

func (vk *valkeyAdapter) Set(ctx context.Context, key string, value string, ttl time.Duration) (setable bool) {
	if vk.keyPrefix != nil {
		key = *vk.keyPrefix + key
	}

	c := vk.primClient
	cmd := c.B().Set().Key(key).Value(value)
	if ttl > 0 {
		// Execute command with TTL
		err := c.Do(ctx, cmd.Ex(ttl).Build()).Error()
		if err != nil {
			slog.ErrorContext(ctx, "valkey.Set", slog.Any("err", err))
		}
		return err == nil
	}

	err := c.Do(ctx, cmd.Build()).Error()
	if err != nil {
		slog.ErrorContext(ctx, "valkey.Set", slog.Any("err", err))
		return false
	}
	return true
}
