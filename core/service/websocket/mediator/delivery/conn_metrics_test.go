package delivery

import (
	"testing"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/stretchr/testify/require"
)

func TestAuthKind(t *testing.T) {
	require.Equal(t, metrics.AuthUser, authKind(voAuth.UserWebsocketAuth("u1", "i1")))
	require.Equal(t, metrics.AuthAnon, authKind(voAuth.AnonymousUserWebsocketAuth("i1")))
}

func TestConnTracker_ReturnsElapsed(t *testing.T) {
	tr := newConnTracker()
	auth := voAuth.UserWebsocketAuth("u1", "i1")

	tr.open(auth)
	time.Sleep(10 * time.Millisecond)
	d, ok := tr.close(auth)
	require.True(t, ok)
	require.GreaterOrEqual(t, d, 10*time.Millisecond)
}

func TestConnTracker_CloseWithoutOpen(t *testing.T) {
	tr := newConnTracker()
	_, ok := tr.close(voAuth.UserWebsocketAuth("u1", "i1"))
	require.False(t, ok, "close without open must report not-found, not a bogus duration")
}

func TestConnTracker_CloseIsIdempotent(t *testing.T) {
	tr := newConnTracker()
	auth := voAuth.UserWebsocketAuth("u1", "i1")

	tr.open(auth)
	_, ok := tr.close(auth)
	require.True(t, ok)

	// Second close must not find an entry — otherwise the map leaks.
	_, ok = tr.close(auth)
	require.False(t, ok)
}

func TestConnTracker_DistinctSessions(t *testing.T) {
	tr := newConnTracker()
	a := voAuth.UserWebsocketAuth("u1", "i1")
	b := voAuth.UserWebsocketAuth("u1", "i2")

	tr.open(a)
	tr.open(b)
	_, okA := tr.close(a)
	_, okB := tr.close(b)
	require.True(t, okA)
	require.True(t, okB)
}
