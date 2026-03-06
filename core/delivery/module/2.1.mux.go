package moduledelivery

import (
	"net/http"
)

func (m *moduleDelivery) Mux() *http.ServeMux {
	return m.mux
}

func (m *moduleDelivery) registerHandlers() {
	m.mux.Handle("/",
		m.WsMiddlewares(m.wsDeli.Mux()))
}
