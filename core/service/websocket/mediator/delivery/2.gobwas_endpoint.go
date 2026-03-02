package delivery

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gobwas/ws"
	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
)

// GobwasEndpoint handles /gw
// Upgrades HTTP connection to WebSocket using gobwas library
func (d *serverDelivery) GobwasEndpoint() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			auth voAuth.WebsocketAuth
			err  error
		)

		// 1. Get connection token from query parameter
		connToken := r.URL.Query().Get("tk")
		fmt.Println("ConnToken: ", connToken)
		switch connToken {
		case "":
			http.Error(w, "Missing connection token", http.StatusUnauthorized)
			return

		default:
			// Scan temporary connection token
			auth, err = d.exchangeToken.ScanConnToken(r.Context(), connToken)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
		}

		// 2. Upgrade HTTP connection to WebSocket
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			slog.Warn("Failed to upgrade connection", slog.Any("error", err))
			http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
			return
		}

		// 3. Create WebSocket connection wrapper
		wsConn, aErr := d.gobwasServer.NewConnection(conn, auth)
		if aErr != nil {
			slog.Error("Failed to create WebSocket connection", slog.Any("error", aErr))
			http.Error(w, aErr.Error(), http.StatusInternalServerError)
			return
		}

		// 4. Handle new connection (register, persist to DynamoDB)
		if err := d.onNewStuff.Do(wsConn); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Connection is now active and will be handled by gobwas server
		slog.Info("New WebSocket connection established",
			slog.Any("auth", auth),
			slog.String("remote_addr", r.RemoteAddr))
	})
}
