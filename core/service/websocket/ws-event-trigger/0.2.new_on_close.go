package wseventtrigger

import (
	"sync"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

func NewOnCloseStuff(c configprovider.ConfigStore) wsSv.OnCloseStuffFn {
	instance := &onCloseStuffFn{
		c:        c,
		fnsMap:   make(map[string]func(auth voAuth.WebsocketAuth)),
		authsMap: make(map[string]voAuth.WebsocketAuth),
	}

	return instance
}

type onCloseStuffFn struct {
	c configprovider.ConfigStore

	mu       sync.RWMutex
	fnsMap   map[string]func(auth voAuth.WebsocketAuth)
	authsMap map[string]voAuth.WebsocketAuth
	fnsAll   []func(auth voAuth.WebsocketAuth)
}

func authKey(auth voAuth.WebsocketAuth) string {
	return auth.UserID + ":" + auth.InstanceID
}

// GetStuffs returns the auths that need to be processed on close
func (o *onCloseStuffFn) GetStuffs() []voAuth.WebsocketAuth {
	o.mu.RLock()
	defer o.mu.RUnlock()
	res := make([]voAuth.WebsocketAuth, 0, len(o.authsMap))
	for _, auth := range o.authsMap {
		res = append(res, auth)
	}
	return res
}

func (o *onCloseStuffFn) Register(auth voAuth.WebsocketAuth, fn func(auth voAuth.WebsocketAuth)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	key := authKey(auth)
	o.fnsMap[key] = fn
	o.authsMap[key] = auth
}

func (o *onCloseStuffFn) RegisterAll(fn func(auth voAuth.WebsocketAuth)) {
	o.fnsAll = append(o.fnsAll, fn)
}

// When complete, auto remove from map
func (o *onCloseStuffFn) Do(auth voAuth.WebsocketAuth) {
	key := authKey(auth)
	o.mu.Lock()
	fn, ok := o.fnsMap[key]
	if ok {
		delete(o.fnsMap, key)
		delete(o.authsMap, key)
	}
	o.mu.Unlock()

	if ok && fn != nil {
		fn(auth)
	}

	for _, allFn := range o.fnsAll {
		if allFn != nil {
			allFn(auth)
		}
	}
}
