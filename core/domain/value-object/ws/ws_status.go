package voWs

type (
	WsStatus int8
)

const (
	WsStatusConnected WsStatus = iota + 1
	WsStatusTempDisconnected
	WsStatusTransferring
)
