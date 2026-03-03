package moduledelivery

import (
	"context"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

func (m *moduleDelivery) SetFns(fns *configprovider.Fns) {
	m.c.SetFns(fns)
	if fns.OnNewConnection != nil {
		m.wsOnNewReg.Register(
			wsSv.OnNewWsKeyName("FnsProvided"),
			func(connection wsSv.WebsocketConn) error {
				return fns.OnNewConnection.OnNewConnection(context.Background(), connection.Auth())
			})
	}
}
