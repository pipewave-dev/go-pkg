package websocket

import (
	"context"

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
	Send(ctx context.Context, payload []byte) error
	CoreType() voWs.WsCoreType
	Close()
	Ping()
}

// CloseWithReasonConn extends WebsocketConn with the ability to close the
// underlying transport with a specific close code and human-readable reason,
// rather than the generic Close(). Only transports that speak a real close
// handshake (e.g. the gobwas WebSocket server) implement this; callers must
// type-assert and fall back to Close() (or logging) if unsupported.
type CloseWithReasonConn interface {
	WebsocketConn
	// CloseWithReason sends a close frame with the given code/reason (best
	// effort) and then tears down the connection.
	CloseWithReason(code uint16, reason string)
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
	SendDirect(ctx context.Context, payload []byte) error
}
