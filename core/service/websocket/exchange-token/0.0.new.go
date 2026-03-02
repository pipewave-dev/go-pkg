package exchangetoken

import (
	"github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/pkg/cache"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
)

const (
	tokenTTL = 10 // 10s
)

type exchangeToken struct {
	obs   observer.Observability
	store cache.CacheProvider
}

func New(
	obs observer.Observability,
	store cache.CacheProvider,
) websocket.ExchangeToken {
	instance := &exchangeToken{
		obs:   obs,
		store: store,
	}
	return instance
}
