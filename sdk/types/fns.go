package types

import (
	"context"
	"net/http"
)

// Fns contains injectable functions that can be provided from outside
type Fns struct {
	InspectToken func(ctx context.Context, token string, headers http.Header) (username string, IsAnonymous bool, metadata map[string]string, err error)

	// HandleMessage is a function that handles incoming messages, it receives the auth, inputType and data, and returns outputType, response data and error if any.
	// if outputType is empty, the message will not be sent back to client, otherwise it will be sent back with the outputType as MsgType
	HandleMessage HandlerMessageT

	OnNewConnection   OnNewConnectionT
	OnCloseConnection OnCloseConnectionT
	OnReadError       OnReadErrorT
	OnWriteError      OnWriteErrorT
}

type HandlerMessageT interface {
	HandleMessage(ctx context.Context, auth WebsocketAuth, inputType string, data []byte) (outputType string, res []byte, err error)
}
type OnNewConnectionT interface {
	OnNewConnection(ctx context.Context, auth WebsocketAuth) error
}
type OnCloseConnectionT interface {
	OnCloseConnection(ctx context.Context, auth WebsocketAuth)
}

type OnReadErrorT interface {
	OnReadError(ctx context.Context, auth WebsocketAuth, err error)
}
type OnWriteErrorT interface {
	OnWriteError(ctx context.Context, auth WebsocketAuth, err error)
}

//

type OnReadErrorFn func(ctx context.Context, auth WebsocketAuth, err error)

type OnWriteErrorFn func(ctx context.Context, auth WebsocketAuth, err error)

func (f OnReadErrorFn) OnReadError(ctx context.Context, auth WebsocketAuth, err error) {
	f(ctx, auth, err)
}

func (f OnWriteErrorFn) OnWriteError(ctx context.Context, auth WebsocketAuth, err error) {
	f(ctx, auth, err)
}
