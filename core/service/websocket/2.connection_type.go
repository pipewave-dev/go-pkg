package websocket

import voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"

type WsConnCoreType int8

const (
	WsConnGobwas WsConnCoreType = iota + 1
	WsConnLongPolling
)

type WebsocketConn interface {
	Auth() voAuth.WebsocketAuth
	Send(payload []byte)
	CoreType() voAuth.WsCoreType
	Close()
	Ping()
}
