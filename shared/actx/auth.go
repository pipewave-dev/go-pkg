package actx

import (
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
)

func (a *aContext) SetWebsocketAuth(auth voAuth.WebsocketAuth) {
	a.data.wsAuth = auth
}

func (a *aContext) GetWebsocketAuth() voAuth.WebsocketAuth {
	return a.data.wsAuth
}
