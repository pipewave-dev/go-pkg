package valkeyadapter

import "context"

func (vk *valkeyAdapter) Incr(ctx context.Context, key string) (int64, bool) {
	if vk.keyPrefix != nil {
		key = *vk.keyPrefix + key
	}

	c := vk.repClient
	n, err := c.Do(ctx, c.B().Incr().Key(key).Build()).AsInt64()
	if err != nil {
		return 0, false
	}

	return n, true
}
