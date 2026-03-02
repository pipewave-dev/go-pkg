package delivery

import (
	"context"
	"log/slog"
	"net/http"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
)

// IssueTmpToken handles POST /issue-tmp-token
// Exchanges JWT access token for temporary WebSocket connection token
func (d *serverDelivery) IssueTmpToken() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract and validate JWT token from Authorization header
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

		// 2. Inspect token using config function
		username, isAnonymous, err := d.c.Env().Fns.InspectToken(r.Context(), authHeader)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		var wsAuth voAuth.WebsocketAuth
		if isAnonymous {
			wsAuth = voAuth.AnonymousUserWebsocketAuth(instanceHeader)
		} else {
			wsAuth = voAuth.UserWebsocketAuth(username, instanceHeader)
		}

		// 3. Exchange for temporary connection token (10s TTL)
		connToken, aerr := d.exchangeToken.Exchange(context.Background(), wsAuth)
		if aerr != nil {
			http.Error(w, aerr.Error(), http.StatusInternalServerError)
			return
		}

		// 4. Set cookie for UserID (for sticky sessions if needed)
		protocolHeader := r.Header.Get("x-forwarded-proto")
		cookieSecure := protocolHeader == "https"

		cookie := &http.Cookie{
			Name:     "__pipewave_userid",
			Value:    wsAuth.UserID,
			Path:     "/",
			MaxAge:   300, // 5 minutes
			HttpOnly: true,
			Secure:   cookieSecure,
			SameSite: http.SameSiteStrictMode,
		}
		http.SetCookie(w, cookie)

		// 6. Return connection token as plain text
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte(connToken))
		if err != nil {
			slog.Error("Failed to write response", slog.Any("error", err))
		}
	})
}
