package delivery

import (
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
)

const maxLPBodySize = 1 << 20 // 1 MB

// LongPollingSendEndpoint handles POST /lp-send
//
// Allows LP clients to push messages to the server (client → server direction),
// mirroring what WebSocket clients do by sending frames over the TCP connection.
//
// The message is routed through clientMsgHandler.HandleBinMessage, identical to
// the WS binary frame path, so the rest of the system is unaware of the transport.
func (d *serverDelivery) LongPollingSendEndpoint() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Authenticate.
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
		username, isAnonymous, metadata, err := fns.InspectToken(r.Context(), authHeader)
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

		// 2. Locate the active LP conn for this session.
		existingConn, found := d.connectionMgr.GetConnection(wsAuth)
		if !found {
			http.Error(w, "no active polling session", http.StatusNotFound)
			return
		}
		lpConn, ok := existingConn.(*LongPollingConn)
		if !ok {
			http.Error(w, "session is not a long polling connection", http.StatusBadRequest)
			return
		}
		if atomic.LoadInt32(&lpConn.closed) == 1 {
			http.Error(w, "polling session has been closed", http.StatusGone)
			return
		}

		// 3. Read body.
		defer r.Body.Close()
		body, err := io.ReadAll(io.LimitReader(r.Body, maxLPBodySize))
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		if len(body) == 0 {
			http.Error(w, "empty body", http.StatusBadRequest)
			return
		}

		// 4. Route through the same handler as WS binary frames.
		d.clientMsgHandler.HandleBinMessage(body, wsAuth, lpConn.Send)

		slog.Debug("LP client message received",
			slog.Any("auth", wsAuth),
			slog.Int("bytes", len(body)))

		w.WriteHeader(http.StatusAccepted)
	})
}
