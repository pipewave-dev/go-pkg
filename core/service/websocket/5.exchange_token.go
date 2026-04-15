package websocket

import (
	"context"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

// ExchangeToken create new token from auth token, the new token will be used to connect to websocket server later. It has a short ttl (e.g: 15s), that enough for client to connect to websocket server but not enough for attacker to use it later.
type ExchangeToken interface {
	// From auth token parse to voAuth.WebsocketAuth, then create new conn tmp token and store it in cache with ttl.
	Exchange(ctx context.Context, auth voAuth.WebsocketAuth) (connTmpToken string, aerr aerror.AError)
	// Scan
	ScanConnToken(ctx context.Context, connTmpToken string) (auth voAuth.WebsocketAuth, aerr aerror.AError)
}
