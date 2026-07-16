package valkeyadapter

import (
	"context"
	"log/slog"

	"github.com/valkey-io/valkey-go"
)

// GetDel atomically reads and deletes key using Valkey's GETDEL command, so a value can
// only ever be consumed once even under concurrent readers.
func (vk *valkeyAdapter) GetDel(ctx context.Context, key string) (val string, found bool) {
	if vk.keyPrefix != nil {
		key = *vk.keyPrefix + key
	}
	// GETDEL is a write, so it must go through the primary client (same as Del).
	c := vk.primClient
	val, err := c.Do(ctx, c.B().Getdel().Key(key).Build()).ToString()
	if err != nil {
		if !valkey.IsValkeyNil(err) {
			slog.ErrorContext(ctx, "valkey.GetDel", slog.Any("err", err))
		}
		return "", false
	}

	return val, true
}
