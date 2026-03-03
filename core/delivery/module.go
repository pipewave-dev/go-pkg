package delivery

import (
	"net/http"

	business "github.com/pipewave-dev/go-pkg/core/service/business"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

// ModuleDelivery is the main interface exposed by pipewave. External Go services embed it as a module.
type ModuleDelivery interface {
	SetFns(fns *configprovider.Fns)

	Mux() *http.ServeMux
	Services() ExportedServices
	Monitoring() business.Monitoring
	Shutdown()
}

type ExportedServices interface {
	wsSv.WsService
	OnNewRegister() wsSv.OnNewStuffFn
	OnCloseRegister() wsSv.OnCloseStuffFn
}
