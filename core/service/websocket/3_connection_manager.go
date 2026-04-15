package websocket

import (
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
)

type ConnectionManager interface {
	AddConnection(connection WebsocketConn)
	RemoveConnection(auth voAuth.WebsocketAuth)

	GetConnection(auth voAuth.WebsocketAuth) (conn WebsocketConn, ok bool)
	GetAllUserConn(userID string) []WebsocketConn
	GetAllAnonymousConn() []WebsocketConn
	GetAllAuthenticatedConn() []WebsocketConn
	GetAllConnections() []WebsocketConn
}
