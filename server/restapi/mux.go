package restapi

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	"github.com/pipewave-dev/go-pkg/server/webhook"
)

type MuxConfig struct {
	APIKeys   []string
	PublicKey webhook.PublicKeyVerifier
}

// NewAdminMux builds the admin REST API from the public ModuleDelivery
// surface. The container mounts it on the admin listener; Go embedders can
// mount it into their own server (spec: "embedded admin API in scope").
func NewAdminMux(pw delivery.ModuleDelivery, cfg MuxConfig) *http.ServeMux {
	h := &handlers{svc: pw.Services(), mon: pw.Monitoring(), publicKey: cfg.PublicKey}

	api := http.NewServeMux()
	api.HandleFunc("POST /api/v1/messages/session", h.sendToSession)
	api.HandleFunc("POST /api/v1/messages/user", h.sendToUser)
	api.HandleFunc("POST /api/v1/messages/users", h.sendToUsers)
	api.HandleFunc("POST /api/v1/messages/broadcast", h.broadcast)
	api.HandleFunc("DELETE /api/v1/sessions/{user_id}/{instance_id}", h.disconnectSession)
	api.HandleFunc("DELETE /api/v1/sessions/{user_id}", h.disconnectUser)
	api.HandleFunc("GET /api/v1/sessions/{user_id}", h.getUserSessions)
	api.HandleFunc("GET /api/v1/presence/{user_id}", h.checkOnline)
	api.HandleFunc("POST /api/v1/presence/batch", h.checkOnlineBatch)
	api.HandleFunc("POST /api/v1/maintenance/cleanup", h.cleanup)
	api.HandleFunc("GET /api/v1/monitoring/connections", h.monitoringConnections)
	api.HandleFunc("GET /api/v1/monitoring/worker-pool", h.monitoringWorkerPool)
	api.HandleFunc("GET /api/v1/webhook/public-key", h.webhookPublicKey)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", requireAPIKey(cfg.APIKeys, api))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if pw.IsHealthy() {
			writeJSON(w, http.StatusOK, map[string]bool{"healthy": true})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]bool{"healthy": false})
	})
	return mux
}

func requireAPIKey(keys []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const bearerPrefix = "Bearer "
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, bearerPrefix) {
			writeUnauthorized(w)
			return
		}
		got := strings.TrimSpace(strings.TrimPrefix(authz, bearerPrefix))
		if got == "" || !matchAnyKey(got, keys) {
			writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func matchAnyKey(got string, keys []string) bool {
	ok := false
	for _, k := range keys {
		// constant-time per key; iterate all keys regardless of match
		if subtle.ConstantTimeCompare([]byte(got), []byte(k)) == 1 {
			ok = true
		}
	}
	return ok
}
