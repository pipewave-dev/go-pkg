package valkeyadapter

import "context"

func (vk *valkeyAdapter) Incr(ctx context.Context, key string) bool {
	if vk.keyPrefix != nil {
		key = *vk.keyPrefix + key
	}

	c := vk.repClient
	m, err := c.Do(ctx, c.B().Incr().Key(key).Build()).AsBool()
	if err != nil {
		return false
	}

	return m
}
