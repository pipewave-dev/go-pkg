package wseventtrigger

import (
	"sync"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

func NewOnCloseStuff(c configprovider.ConfigStore) wsSv.OnCloseStuffFn {
	instance := &onCloseStuffFn{
		c:      c,
		fnsMap: make(map[voAuth.WebsocketAuth]func(auth voAuth.WebsocketAuth)),
	}

	return instance
}

type onCloseStuffFn struct {
	c configprovider.ConfigStore

	mu     sync.RWMutex
	fnsMap map[voAuth.WebsocketAuth]func(auth voAuth.WebsocketAuth)
	fnAll  func(auth voAuth.WebsocketAuth)
}

// GetStuffs returns the auths that need to be processed on close
func (o *onCloseStuffFn) GetStuffs() []voAuth.WebsocketAuth {
	o.mu.RLock()
	defer o.mu.RUnlock()
	res := make([]voAuth.WebsocketAuth, 0, len(o.fnsMap))
	for auth := range o.fnsMap {
		res = append(res, auth)
	}
	return res
}

func (o *onCloseStuffFn) Register(auth voAuth.WebsocketAuth, fn func(auth voAuth.WebsocketAuth)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.fnsMap[auth] = fn
}

func (o *onCloseStuffFn) RegisterAll(fn func(auth voAuth.WebsocketAuth)) {
	o.fnAll = fn
}

// When complete, auto remove from map
func (o *onCloseStuffFn) Do(auth voAuth.WebsocketAuth) {
	o.mu.Lock()
	fn, ok := o.fnsMap[auth]
	if ok {
		delete(o.fnsMap, auth)
	}
	o.mu.Unlock()

	if ok && fn != nil {
		fn(auth)
	}

	if o.fnAll != nil {
		o.fnAll(auth)
	}
}
