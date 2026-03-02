package websocket

import (
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"golang.org/x/time/rate"
)

type RateLimiter interface {
	Get(auth voAuth.WebsocketAuth) *rate.Limiter
	New(auth voAuth.WebsocketAuth) *rate.Limiter
	Remove(auth voAuth.WebsocketAuth)
}
