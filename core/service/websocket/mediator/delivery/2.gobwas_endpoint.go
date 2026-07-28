package delivery

import (
	"log/slog"
	"net/http"

	"github.com/gobwas/ws"
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
)

// GobwasEndpoint handles /gw
// Upgrades HTTP connection to WebSocket using gobwas library
func (d *serverDelivery) GobwasEndpoint() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			auth voAuth.WebsocketAuth
			err  error
		)
		ctx := r.Context()

		// 1. Get connection token from query parameter
		connToken := r.URL.Query().Get("tk")
		switch connToken {
		case "":
			d.metrics.RecordConnectionRejected(ctx, metrics.TransportWS, metrics.RejectMissingToken)
			http.Error(w, "Missing connection token", http.StatusUnauthorized)
			return

		default:
			// Scan temporary connection token
			auth, err = d.exchangeToken.ScanConnToken(r.Context(), connToken)
			if err != nil {
				d.metrics.RecordConnectionRejected(ctx, metrics.TransportWS, metrics.RejectInvalidToken)
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
		}

		// 2. Upgrade HTTP connection to WebSocket
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			d.metrics.RecordConnectionRejected(ctx, metrics.TransportWS, metrics.RejectUpgradeFailed)
			slog.Warn("Failed to upgrade connection", slog.Any("error", err))
			http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
			return
		}

		// 3. Create WebSocket connection wrapper
		wsConn, aErr := d.gobwasServer.NewConnection(conn, auth)
		if aErr != nil {
			d.metrics.RecordConnectionRejected(ctx, metrics.TransportWS, metrics.RejectRegisterFailed)
			slog.Error("Failed to create WebSocket connection", slog.Any("error", aErr))
			http.Error(w, aErr.Error(), http.StatusInternalServerError)
			return
		}

		// 4. Handle new connection (register, persist to DynamoDB)
		if err := d.onNewStuff.Do(wsConn); err != nil {
			// wsConn was already registered with netpoll by NewConnection above;
			// close it here so the fd isn't leaked (it never reaches
			// ConnectionManager, so nothing else will clean it up).
			d.metrics.RecordConnectionRejected(ctx, metrics.TransportWS, metrics.RejectRegisterFailed)
			wsConn.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Connection is now active and will be handled by gobwas server
		slog.Debug("New WebSocket connection established",
			slog.Any("auth", auth),
			slog.String("remote_addr", r.RemoteAddr))
	})
}
