package exchangetoken

import (
	"github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/pkg/cache"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/samber/do/v2"
)

const (
	tokenTTL = 10 // 10s
)

func NewDI(i do.Injector) (websocket.ExchangeToken, error) {
	obs := do.MustInvoke[observer.Observability](i)
	store := do.MustInvoke[cache.CacheProvider](i)
	instance := &exchangeToken{
		obs:   obs,
		store: store,
	}
	return instance, nil
}

type exchangeToken struct {
	obs   observer.Observability
	store cache.CacheProvider
}
