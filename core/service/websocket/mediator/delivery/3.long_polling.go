package delivery

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	msgpack "github.com/vmihailenco/msgpack/v5"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	"github.com/pipewave-dev/go-pkg/pkg/queue"
)

const (
	lpQueueSize   = 64
	lpIdleTimeout = 5 * time.Second
	lpPollTimeout = 30 * time.Second
	lpChannelTTL  = 600 // 10 minute timeout – auto-cleanup orphaned long-polling channels
)

// LongPollingConn implements wsSv.WebsocketConn for long polling transport.
// Instead of a persistent TCP connection, it holds an in-memory message queue
// that HTTP poll handlers drain on each request.
//
// Lifecycle:
//
//	First GET /lp  → newLongPollingConn() → onNew() registers in ConnectionManager
//	Subsequent GET /lp → reuse same conn, reset idle timer
//	5s without any poll → idleTimer fires → Close() → onClose callback (same as WS)
type LongPollingConn struct {
	auth      voAuth.WebsocketAuth
	queue     queue.Adapter
	channel   string
	done      chan struct{}
	closed    int32 // atomic: 0=open, 1=closed
	mu        sync.Mutex
	drainMu   sync.RWMutex
	idleTimer *time.Timer
	onCloseFn wsSv.OnCloseStuffFn
}

// Compile-time check: LongPollingConn must implement WebsocketConn.
var (
	_ wsSv.WebsocketConn = (*LongPollingConn)(nil)
	_ wsSv.DrainableConn = (*LongPollingConn)(nil)
)

func lpChannelName(auth voAuth.WebsocketAuth) string {
	if auth.IsAnonymous() {
		return fmt.Sprintf("lp:anon:%s", auth.InstanceID)
	}
	return fmt.Sprintf("lp:%s:%s", auth.UserID, auth.InstanceID)
}

func newLongPollingConn(auth voAuth.WebsocketAuth, qa queue.Adapter, onCloseFn wsSv.OnCloseStuffFn) *LongPollingConn {
	return &LongPollingConn{
		auth:      auth,
		queue:     qa,
		channel:   lpChannelName(auth),
		done:      make(chan struct{}),
		onCloseFn: onCloseFn,
	}
}

func (c *LongPollingConn) CoreType() voWs.WsCoreType {
	return voWs.WsCoreLongPolling
}

func (c *LongPollingConn) Auth() voAuth.WebsocketAuth { return c.auth }

// Send publishes payload to the Valkey-backed queue.
func (c *LongPollingConn) Send(ctx context.Context, payload []byte) error {
	c.drainMu.RLock()
	defer c.drainMu.RUnlock()
	if err := c.queue.Publish(ctx, c.channel, payload); err != nil {
		slog.Error("LP conn: failed to publish message", slog.Any("error", err), slog.Any("auth", c.auth))
		return err
	}
	return nil
}

// BeginDrain acquires an exclusive lock, blocking all concurrent Send() calls.
func (c *LongPollingConn) BeginDrain() { c.drainMu.Lock() }

// EndDrain releases the exclusive lock, allowing blocked Send() calls to proceed.
func (c *LongPollingConn) EndDrain() { c.drainMu.Unlock() }

// SendDirect publishes directly to the Valkey queue without acquiring drainMu.
// Must only be called between BeginDrain/EndDrain.
func (c *LongPollingConn) SendDirect(ctx context.Context, payload []byte) error {
	if err := c.queue.Publish(ctx, c.channel, payload); err != nil {
		slog.Error("LP conn: SendDirect failed", slog.Any("error", err), slog.Any("auth", c.auth))
		return err
	}
	return nil
}

// Close terminates the connection and triggers the onClose callback exactly once.
func (c *LongPollingConn) Close() {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return
	}
	c.mu.Lock()
	if c.idleTimer != nil {
		c.idleTimer.Stop()
		c.idleTimer = nil
	}
	c.mu.Unlock()
	close(c.done)

	// Messages remaining in the queue (key "lp:<UserID>:<InstanceID>") are NOT
	// deleted here. The channel TTL (set via SetChannelTTL on each poll) will
	// auto-expire it. If the same client reconnects before the TTL elapses, it
	// can still recover buffered messages.
	err := c.queue.SetChannelTTL(context.Background(), c.channel, 60)
	if err != nil {
		slog.Error(fmt.Sprintf("Fail to set TTL for channel %s", c.channel))
	}

	c.onCloseFn.Do(c.auth)
}

// Ping is a no-op. Dead-client detection is handled via the idle timer.
func (c *LongPollingConn) Ping() {}

// stopIdleTimer pauses the idle watchdog while a poll request is in flight.
// Called at the start of each poll so the timer does not fire mid-request.
func (c *LongPollingConn) stopIdleTimer() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idleTimer != nil {
		c.idleTimer.Stop()
		c.idleTimer = nil
	}
}

// resetIdleTimer restarts the 5s watchdog after each poll response is sent.
// If no new poll arrives within lpIdleTimeout, Close() fires automatically.
func (c *LongPollingConn) resetIdleTimer() {
	if atomic.LoadInt32(&c.closed) == 1 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idleTimer != nil {
		c.idleTimer.Stop()
	}
	c.idleTimer = time.AfterFunc(lpIdleTimeout, c.Close)
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// LongPollingEndpoint handles GET /lp
//
// First request (no existing conn in ConnectionManager):
//   - Authenticates via Authorization header
//   - Creates LongPollingConn → calls onNew() (persists to DynamoDB, registers in memory)
//
// Subsequent requests (LP conn already registered):
//   - Reuses the existing LongPollingConn
//   - Resets the idle watchdog
//
// In both cases the handler blocks until:
//   - At least one message is available → respond 200 JSON array of base64 payloads
//   - 30s timeout → respond 204 No Content (client should poll again immediately)
//   - Connection closed by server → respond 410 Gone
func (d *serverDelivery) LongPollingEndpoint() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Authenticate via Bearer token in Authorization header.
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
			return
		}
		instanceHeader := r.Header.Get("X-Pipewave-ID")
		if instanceHeader == "" {
			http.Error(w, "Missing X-Pipewave-ID header", http.StatusBadRequest)
			return
		}

		fns := d.c.Env().Fns
		if fns == nil || fns.InspectToken == nil {
			panic("InspectToken function is not implemented")
		}
		username, isAnonymous, metadata, err := fns.InspectToken(r.Context(), authHeader, r.Header)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		var wsAuth voAuth.WebsocketAuth
		if isAnonymous {
			wsAuth = voAuth.AnonymousUserWebsocketAuthWithMetadata(instanceHeader, metadata)
		} else {
			wsAuth = voAuth.UserWebsocketAuthWithMetadata(username, instanceHeader, metadata)
		}

		// 2. Detect first poll vs. reconnect using ConnectionManager.
		var lpConn *LongPollingConn
		existingConn, found := d.connectionMgr.GetConnection(wsAuth)
		switch {
		case !found:
			// First poll: create LP conn and register it (mirrors GobwasEndpoint's onNew flow).
			lpConn = newLongPollingConn(wsAuth, d.queueAdapter, d.onCloseStuff)
			if err := d.onNewStuff.Do(lpConn); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			slog.Debug("New long polling connection established",
				slog.Any("auth", wsAuth),
				slog.String("remote_addr", r.RemoteAddr))

		case isLPConn(existingConn):
			// Subsequent poll: reuse existing LP conn.
			lpConn = existingConn.(*LongPollingConn)

		default:
			// A WebSocket connection already owns this session.
			http.Error(w, "active WebSocket connection exists for this session", http.StatusConflict)
			return
		}

		// 3. Pause idle watchdog while this poll is in flight.
		//    Prevents the 5s timer (started after the previous response) from
		//    firing during the up-to-30s drainOrWait window.
		lpConn.stopIdleTimer()

		// 4. Block until messages arrive, timeout, or connection closed.
		ctx, cancel := context.WithTimeout(r.Context(), lpPollTimeout)
		defer cancel()

		msgs := drainOrWait(ctx, lpConn)

		// 5. Reset idle watchdog before writing response so the timer accounts
		//    for the round-trip time the client needs to reconnect.
		lpConn.resetIdleTimer()

		// Refresh channel TTL on each poll so orphaned queues are auto-cleaned.
		if err = lpConn.queue.SetChannelTTL(context.Background(), lpConn.channel, lpChannelTTL); err != nil {
			slog.Error("LP: failed to set channel TTL", slog.Any("error", err))
		}

		// 6. Write response.
		if msgs == nil {
			if atomic.LoadInt32(&lpConn.closed) == 1 {
				w.WriteHeader(http.StatusGone) // 410: server closed the session
				return
			}
			w.WriteHeader(http.StatusNoContent) // 204: timeout, no messages
			return
		}

		writeLPResponse(w, msgs)
	})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// drainOrWait drains any messages already in the Valkey-backed queue.
// If the queue is empty it blocks (via BLPOP) until the first message arrives
// or the context expires. Returns nil when no message was received.
func drainOrWait(ctx context.Context, lpConn *LongPollingConn) [][]byte {
	// Fast path: drain whatever is already buffered.
	msgs, _ := lpConn.queue.FetchMany(ctx, lpConn.channel, lpQueueSize)
	if len(msgs) > 0 {
		return msgs
	}

	// Slow path: blocking wait for first message.
	msg, err := lpConn.queue.BlockFetchOne(ctx, lpConn.channel, lpPollTimeout)
	if err != nil || msg == nil {
		return nil
	}
	msgs = [][]byte{msg}

	// Drain more that arrived concurrently.
	more, _ := lpConn.queue.FetchMany(ctx, lpConn.channel, lpQueueSize-1)
	return append(msgs, more...)
}

// writeLPResponse encodes msgs as a msgpack binary array and writes it.
// Using msgpack instead of JSON+base64 avoids the base64 overhead and produces
// a smaller payload, reducing long-polling transfer latency.
func writeLPResponse(w http.ResponseWriter, msgs [][]byte) {
	w.Header().Set("Content-Type", "application/x-msgpack")
	w.WriteHeader(http.StatusOK)
	if err := msgpack.NewEncoder(w).Encode(msgs); err != nil {
		slog.Error("Failed to write LP response", slog.Any("error", err))
	}
}

// isLPConn returns true if conn is a *LongPollingConn.
func isLPConn(conn wsSv.WebsocketConn) bool {
	_, ok := conn.(*LongPollingConn)
	return ok
}
