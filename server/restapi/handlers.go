package restapi

import (
	"net/http"
	"time"

	"github.com/pipewave-dev/go-pkg/core/delivery"
	business "github.com/pipewave-dev/go-pkg/core/service/business"
	"github.com/pipewave-dev/go-pkg/server/webhook"
)

type handlers struct {
	svc       delivery.ExportedServices
	mon       business.Monitoring
	publicKey webhook.PublicKeyVerifier
}

type sendResult struct {
	Sent  *bool `json:"sent,omitempty"`
	Acked *bool `json:"acked,omitempty"`
}

func sentResult() sendResult         { v := true; return sendResult{Sent: &v} }
func ackedResult(ok bool) sendResult { return sendResult{Acked: &ok} }

type sendToSessionReq struct {
	UserID       string `json:"user_id"`
	InstanceID   string `json:"instance_id"`
	MsgType      string `json:"msg_type"`
	Payload      []byte `json:"payload"`
	AckTimeoutMs int    `json:"ack_timeout_ms"`
}

func (h *handlers) sendToSession(w http.ResponseWriter, r *http.Request) {
	var req sendToSessionReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.UserID == "" || req.InstanceID == "" || req.MsgType == "" {
		writeBadRequest(w, "user_id, instance_id and msg_type are required")
		return
	}
	if req.AckTimeoutMs > 0 {
		acked, aErr := h.svc.SendToSessionWithAck(r.Context(), req.UserID, req.InstanceID, req.MsgType, req.Payload,
			time.Duration(req.AckTimeoutMs)*time.Millisecond)
		if aErr != nil {
			writeAError(w, aErr)
			return
		}
		writeJSON(w, http.StatusOK, ackedResult(acked))
		return
	}
	if aErr := h.svc.SendToSession(r.Context(), req.UserID, req.InstanceID, req.MsgType, req.Payload); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, sentResult())
}

type sendToUserReq struct {
	UserID       string `json:"user_id"`
	MsgType      string `json:"msg_type"`
	Payload      []byte `json:"payload"`
	AckTimeoutMs int    `json:"ack_timeout_ms"`
}

func (h *handlers) sendToUser(w http.ResponseWriter, r *http.Request) {
	var req sendToUserReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.UserID == "" || req.MsgType == "" {
		writeBadRequest(w, "user_id and msg_type are required")
		return
	}
	if req.AckTimeoutMs > 0 {
		acked, aErr := h.svc.SendToUserWithAck(r.Context(), req.UserID, req.MsgType, req.Payload,
			time.Duration(req.AckTimeoutMs)*time.Millisecond)
		if aErr != nil {
			writeAError(w, aErr)
			return
		}
		writeJSON(w, http.StatusOK, ackedResult(acked))
		return
	}
	if aErr := h.svc.SendToUser(r.Context(), req.UserID, req.MsgType, req.Payload); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, sentResult())
}

type sendToUsersReq struct {
	UserIDs []string `json:"user_ids"`
	MsgType string   `json:"msg_type"`
	Payload []byte   `json:"payload"`
}

func (h *handlers) sendToUsers(w http.ResponseWriter, r *http.Request) {
	var req sendToUsersReq
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.UserIDs) == 0 || req.MsgType == "" {
		writeBadRequest(w, "user_ids and msg_type are required")
		return
	}
	if aErr := h.svc.SendToUsers(r.Context(), req.UserIDs, req.MsgType, req.Payload); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, sentResult())
}

type broadcastReq struct {
	Target      string   `json:"target"` // all | authenticated | anonymous
	MsgType     string   `json:"msg_type"`
	Payload     []byte   `json:"payload"`
	InstanceIDs []string `json:"instance_ids"`
}

func (h *handlers) broadcast(w http.ResponseWriter, r *http.Request) {
	var req broadcastReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.MsgType == "" {
		writeBadRequest(w, "msg_type is required")
		return
	}
	switch req.Target {
	case "all":
		if e := h.svc.SendToAll(r.Context(), req.MsgType, req.Payload); e != nil {
			writeAError(w, e)
			return
		}
	case "authenticated":
		if e := h.svc.SendToAuthenticated(r.Context(), req.MsgType, req.Payload); e != nil {
			writeAError(w, e)
			return
		}
	case "anonymous":
		if e := h.svc.SendToAnonymous(r.Context(), req.MsgType, req.Payload, len(req.InstanceIDs) == 0, req.InstanceIDs); e != nil {
			writeAError(w, e)
			return
		}
	default:
		writeBadRequest(w, `target must be one of "all", "authenticated", "anonymous"`)
		return
	}
	writeJSON(w, http.StatusOK, sentResult())
}

func (h *handlers) disconnectSession(w http.ResponseWriter, r *http.Request) {
	if aErr := h.svc.DisconnectSession(r.Context(), r.PathValue("user_id"), r.PathValue("instance_id")); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"disconnected": true})
}

func (h *handlers) disconnectUser(w http.ResponseWriter, r *http.Request) {
	if aErr := h.svc.DisconnectUser(r.Context(), r.PathValue("user_id")); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"disconnected": true})
}

func (h *handlers) checkOnline(w http.ResponseWriter, r *http.Request) {
	online, aErr := h.svc.CheckOnline(r.Context(), r.PathValue("user_id"))
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"online": online})
}

type presenceBatchReq struct {
	UserIDs []string `json:"user_ids"`
}

func (h *handlers) checkOnlineBatch(w http.ResponseWriter, r *http.Request) {
	var req presenceBatchReq
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.UserIDs) == 0 {
		writeBadRequest(w, "user_ids is required")
		return
	}
	results, aErr := h.svc.CheckOnlineMultiple(r.Context(), req.UserIDs)
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

type sessionDTO struct {
	InstanceID     string    `json:"instance_id"`
	HolderID       string    `json:"holder_id"`
	ConnectionType string    `json:"connection_type"`
	Status         string    `json:"status"`
	ConnectedAt    time.Time `json:"connected_at"`
}

func (h *handlers) getUserSessions(w http.ResponseWriter, r *http.Request) {
	sessions, aErr := h.svc.GetUserSessions(r.Context(), r.PathValue("user_id"))
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	out := make([]sessionDTO, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionDTO{
			InstanceID:     s.InstanceID,
			HolderID:       s.HolderID,
			ConnectionType: s.ConnectionType.String(),
			Status:         s.Status.String(),
			ConnectedAt:    s.ConnectedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (h *handlers) cleanup(w http.ResponseWriter, r *http.Request) {
	if aErr := h.svc.CleanUp(r.Context()); aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *handlers) monitoringConnections(w http.ResponseWriter, r *http.Request) {
	inside, aErr := h.mon.InsideActiveConnection(r.Context())
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	total, aErr := h.mon.TotalActiveConnection(r.Context())
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"inside": map[string]int{
			"anonymous_connections": inside.AnonymosConnection,
			"user_connections":      inside.UserConnection,
			"total_users":           inside.TotalUser,
		},
		"total": total,
	})
}

func (h *handlers) monitoringWorkerPool(w http.ResponseWriter, r *http.Request) {
	stats, aErr := h.mon.WorkerPoolStats(r.Context())
	if aErr != nil {
		writeAError(w, aErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"length":   stats.Length,
		"capacity": stats.Capacity,
		"dropped":  stats.Dropped,
	})
}

func (h *handlers) webhookPublicKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.publicKey)
}
