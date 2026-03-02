package valkeyadapter

import (
	"context"
	"log/slog"

	"github.com/valkey-io/valkey-go"
)

func (vk *valkeyAdapter) Get(ctx context.Context, key string) (val string, found bool) {
	if vk.keyPrefix != nil {
		key = *vk.keyPrefix + key
	}
	// get value from cache
	c := vk.repClient
	val, err := c.Do(ctx, c.B().Get().Key(key).Build()).ToString()
	if err != nil {
		if !valkey.IsValkeyNil(err) {
			slog.ErrorContext(ctx, "valkey.Get", slog.Any("err", err))
		}
		return "", false
	}

	return val, true
}
