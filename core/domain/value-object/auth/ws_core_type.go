package voAuth

type (
	WsCoreType int8
)

const (
	WsCoreGobwas WsCoreType = iota + 1
	WsCoreLongPolling
)
