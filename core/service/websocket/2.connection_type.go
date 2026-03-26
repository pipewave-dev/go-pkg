package websocket

import (
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
)

type WsConnCoreType int8

const (
	WsConnGobwas WsConnCoreType = iota + 1
	WsConnLongPolling
)

type WebsocketConn interface {
	Auth() voAuth.WebsocketAuth
	Send(payload []byte)
	CoreType() voWs.WsCoreType
	Close()
	Ping()
}
