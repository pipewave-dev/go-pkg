package configprovider

import (
	"context"
	"fmt"

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

// LoadDefault sets default implementations for all functions if they are nil
func (f *Fns) LoadDefault() {
	if f.InspectToken == nil {
		f.InspectToken = func(ctx context.Context, token string) (string, bool, error) {
			return "", false, fmt.Errorf("InspectToken function is not implemented")
		}
	}
	if f.HandleMessage == nil {
		panic("HandleMessage function can not be null")
	}
}

func (f *Fns) Validate() {
	if f.InspectToken == nil {
		panic("InspectToken function can not be null")
	}
	if f.HandleMessage == nil {
		panic("HandleMessage function can not be null")
	}
}
