package valkeyadapter

import (
	"context"
	"log/slog"
)

func (vk *valkeyAdapter) Keys(ctx context.Context, pattern string) []string {
	if vk.keyPrefix != nil {
		pattern = *vk.keyPrefix + pattern
	}

	c := vk.repClient

	m, err := c.Do(ctx, c.B().Keys().Pattern(pattern).Build()).AsStrSlice()
	if err != nil {
		slog.ErrorContext(ctx, "valkey.Keys", slog.Any("err", err))
		return []string{}
	}

	return m
}
