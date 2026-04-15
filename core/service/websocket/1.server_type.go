package websocket

import (
	"context"
	"net"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

type WebsocketServer interface {
	// OnTextMessage(f OnTextMessageFn)
	// OnBinMessage(f OnBinMessageFn)
	// OnReadError(f OnReadErrorFn)
	// OnWriteError(f OnWriteErrorFn)
	// OnClose(f OnCloseFn)

	NewConnection(
		conn net.Conn,
		propAuth voAuth.WebsocketAuth,
	) (wsConn WebsocketConn, aErr aerror.AError)
}

type (
	OnTextMessageFn func(
		ctx context.Context,
		payload string,
		auth voAuth.WebsocketAuth,
		sendFn func(ctx context.Context, payload []byte) error,
	)
	OnBinMessageFn func(
		ctx context.Context,
		payload []byte,
		auth voAuth.WebsocketAuth,
		sendFn func(ctx context.Context, payload []byte) error,
	)
	OnReadErrorFn  func(ctx context.Context, auth voAuth.WebsocketAuth, err error)
	OnWriteErrorFn func(ctx context.Context, auth voAuth.WebsocketAuth, err error)
)

type OnCloseStuffFn interface {
	GetStuffs() []voAuth.WebsocketAuth // GetStuffs returns the auths that need to be processed on close

	// Register registers the function to be called on close event for a given auth
	Register(auth voAuth.WebsocketAuth, fn func(auth voAuth.WebsocketAuth))

	RegisterAll(fn func(auth voAuth.WebsocketAuth))

	// Do function executes the registered function for the given auth and removes it from the map
	Do(auth voAuth.WebsocketAuth)
}

type OnNewWsKeyName string

type OnNewStuffFn interface {
	Register(key OnNewWsKeyName, fn func(conn WebsocketConn) error)
	Deregister(key OnNewWsKeyName)

	Do(conn WebsocketConn) error
}

type ClientMsgHandler interface {
	HandleTextMessage(ctx context.Context, payload string, auth voAuth.WebsocketAuth, sendFn func(ctx context.Context, payload []byte) error)
	HandleBinMessage(ctx context.Context, payload []byte, auth voAuth.WebsocketAuth, sendFn func(ctx context.Context, payload []byte) error)
}
