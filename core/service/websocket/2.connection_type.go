package websocket

import voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"

type WebsocketConn interface {
	Auth() voAuth.WebsocketAuth
	Send(payload []byte)
	Close()
	Ping()
}
