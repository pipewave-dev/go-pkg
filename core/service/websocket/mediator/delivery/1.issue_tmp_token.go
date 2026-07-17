package delivery

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
)

// issueTmpTokenResponse is the JSON body returned by POST /issue-tmp-token.
// InstanceID is only populated for anonymous sessions: it's the server-issued,
// HMAC-signed token the client must store and replay as X-Pipewave-ID on
// subsequent /issue-tmp-token, /lp and /lp-send calls (see anon_instance.go).
type issueTmpTokenResponse struct {
	ConnToken  string `json:"connToken"`
	InstanceID string `json:"instanceId,omitempty"`
}

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

		// This is the sole mint point for anonymous InstanceIDs (see anon_instance.go),
		// so throttle it per client IP regardless of auth outcome.
		ip := clientIP(r, d.c.Env().ExtractHeader.IpHeader)
		if !d.issueTokenIPLimiter.Allow(ip) {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}

		// 2. Inspect token using config function
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
		var anonInstanceID string
		if isAnonymous {
			// Server-issued, not client-controlled: closes the rate-limit bypass and
			// session takeover vectors in ai-feedback/06-anonymous-ratelimit-session-bypass.md.
			anonInstanceID = d.anonInstanceSigner.mintOrReadAnonymousInstanceID(r)
			wsAuth = voAuth.AnonymousUserWebsocketAuthWithMetadata(anonInstanceID, metadata)
		} else {
			instanceHeader := r.Header.Get("X-Pipewave-ID")
			if instanceHeader == "" {
				http.Error(w, "Missing X-Pipewave-ID header", http.StatusBadRequest)
				return
			}
			wsAuth = voAuth.UserWebsocketAuthWithMetadata(username, instanceHeader, metadata)
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
			Name:     "__pw_uid",
			Value:    wsAuth.UserID,
			Path:     "/",
			MaxAge:   300, // 5 minutes
			HttpOnly: true,
			Secure:   cookieSecure,
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, cookie)

		// 6. Return connection token (+ signed anonymous instance id, if any) as JSON.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := issueTmpTokenResponse{ConnToken: connToken, InstanceID: anonInstanceID}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to write response", slog.Any("error", err))
		}
	})
}
