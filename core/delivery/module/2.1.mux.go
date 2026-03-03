package moduledelivery

import (
	"net/http"
)

func (m *moduleDelivery) Mux() *http.ServeMux {
	return m.mux
}

func (m *moduleDelivery) registerHandlers() {
	m.mux.Handle("/websocket/",
		m.WsMiddlewares(http.StripPrefix("/websocket", m.wsDeli.Mux())))
}
