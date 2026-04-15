package voWs

type (
	WsStatus int8
)

const (
	WsStatusConnected WsStatus = iota + 1
	WsStatusTempDisconnected
	WsStatusTransferring
)

func (s WsStatus) String() string {
	switch s {
	case WsStatusConnected:
		return "connected"
	case WsStatusTempDisconnected:
		return "temp_disconnected"
	case WsStatusTransferring:
		return "transferring"
	default:
		return "unknown"
	}
}
