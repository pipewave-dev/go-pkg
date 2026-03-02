package actx

import (
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
)

func (a *aContext) SetAuth(auth voAuth.Auth) {
	a.data.auth = auth
}

func (a *aContext) GetAuth() voAuth.Auth {
	return a.data.auth
}
