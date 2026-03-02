package valkeyadapter

import "context"

func (vk *valkeyAdapter) Decr(ctx context.Context, key string) bool {
	if vk.keyPrefix != nil {
		key = *vk.keyPrefix + key
	}

	c := vk.repClient
	m, err := c.Do(ctx, c.B().Decr().Key(key).Build()).AsBool()
	if err != nil {
		return false
	}

	return m
}
