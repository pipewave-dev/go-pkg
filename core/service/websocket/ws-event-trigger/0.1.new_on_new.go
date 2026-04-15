package wseventtrigger

import (
	"sync"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/samber/do/v2"
)

func NewDIOnNewStuff(i do.Injector) (wsSv.OnNewStuffFn, error) {
	c := do.MustInvoke[configprovider.ConfigStore](i)
	instance := &onNewStuffFn{
		c:      c,
		fnsMap: make(map[wsSv.OnNewWsKeyName]func(conn wsSv.WebsocketConn) error),
	}

	return instance, nil
}

func NewOnNewStuff(c configprovider.ConfigStore) wsSv.OnNewStuffFn {
	instance := &onNewStuffFn{
		c:      c,
		fnsMap: make(map[wsSv.OnNewWsKeyName]func(conn wsSv.WebsocketConn) error),
	}

	return instance
}

type onNewStuffFn struct {
	c configprovider.ConfigStore

	mu     sync.RWMutex
	fnsMap map[wsSv.OnNewWsKeyName]func(conn wsSv.WebsocketConn) error
}

func (o *onNewStuffFn) Register(key wsSv.OnNewWsKeyName, fn func(conn wsSv.WebsocketConn) error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.fnsMap[key] = fn
}

func (o *onNewStuffFn) Deregister(key wsSv.OnNewWsKeyName) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.fnsMap, key)
}

func (o *onNewStuffFn) Do(conn wsSv.WebsocketConn) error {
	o.mu.RLock()
	fns := make([]func(conn wsSv.WebsocketConn) error, 0, len(o.fnsMap))
	for _, fn := range o.fnsMap {
		fns = append(fns, fn)
	}
	o.mu.RUnlock()

	for _, fn := range fns {
		if err := fn(conn); err != nil {
			return err
		}
	}

	return nil
}
