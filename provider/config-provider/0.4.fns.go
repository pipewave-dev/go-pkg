package configprovider

import (
	"context"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
)

// Fns contains injectable functions that can be provided from outside
type Fns struct {
	InspectToken func(ctx context.Context, token string) (username string, IsAnonymous bool, err error)

	// HandleMessage is a function that handles incoming messages, it receives the auth, inputType and data, and returns outputType, response data and error if any.
	// if outputType is empty, the message will not be sent back to client, otherwise it will be sent back with the outputType as MsgType
	HandleMessage HandlerMessageT

	OnNewConnection   OnNewConnectionT
	OnCloseConnection OnCloseConnectionT
}

type HandlerMessageT interface {
	HandleMessage(ctx context.Context, auth voAuth.WebsocketAuth, inputType string, data []byte) (outputType string, res []byte, err error)
}
type OnNewConnectionT interface {
	OnNewConnection(ctx context.Context, auth voAuth.WebsocketAuth) error
}
type OnCloseConnectionT interface {
	OnCloseConnection(ctx context.Context, auth voAuth.WebsocketAuth)
}
