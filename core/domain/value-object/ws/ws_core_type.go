package voWs

type (
	WsCoreType int8
)

const (
	WsCoreGobwas WsCoreType = iota + 1
	WsCoreLongPolling
)

func (w WsCoreType) String() string {
	switch w {
	case WsCoreGobwas:
		return "gobwas"
	case WsCoreLongPolling:
		return "long_polling"
	default:
		return "unknown"
	}
}
