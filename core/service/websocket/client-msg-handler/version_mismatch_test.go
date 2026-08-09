package clientmsghandler

import (
	"context"
	"testing"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/stretchr/testify/require"
)

// fakeConnectionManager is a minimal wsSv.ConnectionManager stand-in that
// only implements what closeOnVersionMismatch touches: GetConnection.
type fakeConnectionManager struct {
	conn wsSv.WebsocketConn
	ok   bool
}

func (f *fakeConnectionManager) AddConnection(wsSv.WebsocketConn)      {}
func (f *fakeConnectionManager) RemoveConnection(voAuth.WebsocketAuth) {}
func (f *fakeConnectionManager) GetConnection(voAuth.WebsocketAuth) (wsSv.WebsocketConn, bool) {
	return f.conn, f.ok
}
func (f *fakeConnectionManager) GetAllUserConn(string) []wsSv.WebsocketConn    { return nil }
func (f *fakeConnectionManager) GetAllAnonymousConn() []wsSv.WebsocketConn     { return nil }
func (f *fakeConnectionManager) GetAllAuthenticatedConn() []wsSv.WebsocketConn { return nil }
func (f *fakeConnectionManager) GetAllConnections() []wsSv.WebsocketConn       { return nil }

// closeableConn implements wsSv.CloseWithReasonConn and records the
// code/reason it was closed with.
type closeableConn struct {
	closedCode   int
	closedReason string
	closeCalled  bool
	plainClosed  bool
}

func (c *closeableConn) Auth() voAuth.WebsocketAuth         { return voAuth.WebsocketAuth{} }
func (c *closeableConn) Send(context.Context, []byte) error { return nil }
func (c *closeableConn) CoreType() voWs.WsCoreType          { return voWs.WsCoreGobwas }
func (c *closeableConn) Ping()                              {}
func (c *closeableConn) Close()                             { c.plainClosed = true }
func (c *closeableConn) CloseWithReason(code uint16, reason string) {
	c.closeCalled = true
	c.closedCode = int(code)
	c.closedReason = reason
}

// plainConn implements only wsSv.WebsocketConn (no coded close support),
// modeling a transport like long polling.
type plainConn struct {
	plainClosed bool
}

func (c *plainConn) Auth() voAuth.WebsocketAuth         { return voAuth.WebsocketAuth{} }
func (c *plainConn) Send(context.Context, []byte) error { return nil }
func (c *plainConn) CoreType() voWs.WsCoreType          { return voWs.WsCoreGobwas }
func (c *plainConn) Ping()                              {}
func (c *plainConn) Close()                             { c.plainClosed = true }

// Compile-time checks.
var (
	_ wsSv.ConnectionManager   = (*fakeConnectionManager)(nil)
	_ wsSv.CloseWithReasonConn = (*closeableConn)(nil)
	_ wsSv.WebsocketConn       = (*plainConn)(nil)
)

func TestCloseOnVersionMismatch_ClosesWithDedicatedCodeAndReason(t *testing.T) {
	conn := &closeableConn{}
	h := &clientMsgHandler{
		connectionMgr: &fakeConnectionManager{conn: conn, ok: true},
	}

	h.closeOnVersionMismatch(context.Background(), voAuth.WebsocketAuth{UserID: "u1", InstanceID: "i1"})

	require.True(t, conn.closeCalled, "expected CloseWithReason to be called")
	require.Equal(t, 1002, conn.closedCode, "expected the protocol-error close code")
	require.NotEmpty(t, conn.closedReason)
	require.Contains(t, conn.closedReason, "version")
}

func TestCloseOnVersionMismatch_MissingConnectionDoesNotPanic(t *testing.T) {
	h := &clientMsgHandler{
		connectionMgr: &fakeConnectionManager{ok: false},
	}

	require.NotPanics(t, func() {
		h.closeOnVersionMismatch(context.Background(), voAuth.WebsocketAuth{UserID: "u1", InstanceID: "i1"})
	})
}

// TestCloseOnVersionMismatch_UnsupportedTransportDoesNotPanic pins the
// fallback for transports (e.g. long polling) that don't implement a coded
// close: it must log and return, never panic or silently succeed as if it
// closed anything.
func TestCloseOnVersionMismatch_UnsupportedTransportDoesNotPanic(t *testing.T) {
	conn := &plainConn{}
	h := &clientMsgHandler{
		connectionMgr: &fakeConnectionManager{conn: conn, ok: true},
	}

	require.NotPanics(t, func() {
		h.closeOnVersionMismatch(context.Background(), voAuth.WebsocketAuth{UserID: "u1", InstanceID: "i1"})
	})
	require.False(t, conn.plainClosed, "closeOnVersionMismatch must not fall back to a plain Close() that hides the mismatch as a generic disconnect")
}
