package delivery

import (
	"sync"
	"time"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
)

// authKind maps an auth to the bounded "auth" metric label.
func authKind(auth voAuth.WebsocketAuth) string {
	if auth.IsAnonymous() {
		return metrics.AuthAnon
	}
	return metrics.AuthUser
}

// transportKind maps a connection's core type to the bounded "transport" label.
func transportKind(coreType voWs.WsCoreType) string {
	if coreType == voWs.WsCoreLongPolling {
		return metrics.TransportLongPoll
	}
	return metrics.TransportWS
}

// authKey identifies a session; mirrors ws-event-trigger's keying.
func authKey(auth voAuth.WebsocketAuth) string {
	return auth.UserID + ":" + auth.InstanceID
}

// connTracker records connection open times so close can report a duration.
// WebsocketConn carries no open timestamp, so this is the only place it lives.
type connTracker struct {
	mu     sync.Mutex
	openAt map[string]time.Time
}

func newConnTracker() *connTracker {
	return &connTracker{openAt: make(map[string]time.Time)}
}

func (t *connTracker) open(auth voAuth.WebsocketAuth) {
	t.mu.Lock()
	t.openAt[authKey(auth)] = time.Now()
	t.mu.Unlock()
}

// close removes the entry and returns how long the connection was open.
// ok is false when no matching open was recorded, so callers skip the metric
// rather than reporting a duration measured from the zero time.
func (t *connTracker) close(auth voAuth.WebsocketAuth) (d time.Duration, ok bool) {
	key := authKey(auth)
	t.mu.Lock()
	start, found := t.openAt[key]
	if found {
		delete(t.openAt, key)
	}
	t.mu.Unlock()
	if !found {
		return 0, false
	}
	return time.Since(start), true
}
