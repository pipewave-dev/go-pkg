package websocket

import (
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
)

type WsConnCoreType int8

const (
	WsConnGobwas WsConnCoreType = iota + 1
	WsConnLongPolling
)

type WebsocketConn interface {
	Auth() voAuth.WebsocketAuth
	Send(payload []byte) error
	CoreType() voWs.WsCoreType
	Close()
	Ping()
}

// DrainableConn extends WebsocketConn with drain-phase locking.
// Connections implementing this interface allow callers to block concurrent
// Send() calls while draining pending messages in the correct order.
//
// Usage pattern:
//
//	dc.BeginDrain()            // acquire exclusive lock — all Send() calls block
//	defer dc.EndDrain()        // release lock — blocked Send() calls proceed after pending
//	for _, msg := range pending {
//	    dc.SendDirect(msg)     // write directly, bypasses drainMu to avoid deadlock
//	}
type DrainableConn interface {
	WebsocketConn
	// BeginDrain acquires an exclusive write lock. All concurrent Send() calls block until EndDrain.
	BeginDrain()
	// EndDrain releases the write lock. Blocked Send() calls resume after all SendDirect calls.
	EndDrain()
	// SendDirect writes payload to the underlying transport without acquiring drainMu.
	// MUST only be called between BeginDrain and EndDrain.
	SendDirect(payload []byte) error
}
