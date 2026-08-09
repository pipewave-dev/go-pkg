package serverfns

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/pipewave-dev/go-pkg/export/types"
	"github.com/pipewave-dev/go-pkg/server/callback"
	serverconfig "github.com/pipewave-dev/go-pkg/server/config"
	"github.com/pipewave-dev/go-pkg/server/webhook"
)

type Config struct {
	HandleMessageMode    string // serverconfig.HandleMsgMode*
	HandleMessageTimeout time.Duration
	SyncTimeout          time.Duration
	// InspectTokenOverride, when non-nil, replaces the inspect_token
	// webhook (JWT mode). Signature matches types.Fns.InspectToken.
	InspectTokenOverride func(ctx context.Context, token string, headers http.Header) (string, bool, map[string]string, error)
}

// Wire DTOs — this is the receiver-side contract; keep in lockstep with
// examples/rest-backend and the design doc.

type authDTO struct {
	UserID     string            `json:"user_id"`
	InstanceID string            `json:"instance_id"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func toAuthDTO(a types.WebsocketAuth) authDTO {
	return authDTO{UserID: a.UserID, InstanceID: a.InstanceID, Metadata: a.Metadata}
}

type inspectTokenReq struct {
	Token   string              `json:"token"`
	Headers map[string][]string `json:"headers,omitempty"`
}

type inspectTokenResp struct {
	UserID      string            `json:"user_id"`
	IsAnonymous bool              `json:"is_anonymous"`
	Metadata    map[string]string `json:"metadata"`
}

type handleMessageReq struct {
	Auth      authDTO `json:"auth"`
	InputType string  `json:"input_type"`
	Data      []byte  `json:"data"` // base64 in JSON
}

type handleMessageResp struct {
	OutputType string `json:"output_type"`
	Data       []byte `json:"data"`
}

type authEvent struct {
	Auth authDTO `json:"auth"`
}

type errorEvent struct {
	Auth  authDTO `json:"auth"`
	Error string  `json:"error"`
}

type webhookFns struct {
	sync  *webhook.SyncCaller
	async callback.AsyncTransport
	cfg   Config
}

// deliberateErrorBody is the shape a backend uses to communicate a
// deliberate rejection (4xx) to the end client, e.g. {"error":"banned"}.
type deliberateErrorBody struct {
	Error string `json:"error"`
}

// sanitizeCallError logs the full failure detail (which may embed the
// private callback URL or a raw backend response body) and returns a safe,
// generic error for core to surface to end clients. A 4xx CallError is
// treated as a deliberate application-level rejection: if the backend body
// decodes to {"error": "..."} with a non-empty message, that message is
// surfaced verbatim; otherwise a generic "<hook> rejected" is used. Every
// other failure (5xx, transport, circuit open, decode errors) collapses to
// genericMsg.
func sanitizeCallError(hook string, genericMsg string, err error) error {
	if err == nil {
		return nil
	}
	slog.Error("[fns] callback failed", "hook", hook, "error", err)

	var ce *webhook.CallError
	if errors.As(err, &ce) && ce.Status >= 400 && ce.Status < 500 {
		var body deliberateErrorBody
		if jsonErr := json.Unmarshal(ce.Body, &body); jsonErr == nil && body.Error != "" {
			return errors.New(body.Error)
		}
		return errors.New(hook + " rejected")
	}
	return errors.New(genericMsg)
}

// New builds the *types.Fns that bridges pipewave hooks to callbacks.
// Class-1 (sync) luôn đi qua HTTP webhook; Class-2 (async) đi qua
// transport được truyền vào (webhook hoặc pubsub).
func New(syncCaller *webhook.SyncCaller, async callback.AsyncTransport, cfg Config) *types.Fns {
	w := &webhookFns{sync: syncCaller, async: async, cfg: cfg}
	return &types.Fns{
		InspectToken:      w.inspectToken,
		HandleMessage:     w,
		OnNewConnection:   w,
		OnCloseConnection: w,
		OnReadError:       w,
		OnWriteError:      w,
	}
}

func (w *webhookFns) inspectToken(ctx context.Context, token string, headers http.Header) (string, bool, map[string]string, error) {
	if w.cfg.InspectTokenOverride != nil {
		return w.cfg.InspectTokenOverride(ctx, token, headers)
	}
	var resp inspectTokenResp
	// Any failure (transport, 4xx, 5xx, open breaker) fails closed.
	if err := w.sync.Call(ctx, webhook.EventInspectToken, inspectTokenReq{Token: token, Headers: headers}, w.cfg.SyncTimeout, &resp); err != nil {
		return "", false, nil, sanitizeCallError(webhook.EventInspectToken, "authentication failed", err)
	}
	if !resp.IsAnonymous && resp.UserID == "" {
		// A 200 with an empty, non-anonymous user_id would panic core's
		// auth path — fail closed instead of trusting a malformed backend.
		slog.Error("[fns] callback returned empty user_id for non-anonymous auth", "hook", webhook.EventInspectToken)
		return "", false, nil, errors.New("authentication failed")
	}
	return resp.UserID, resp.IsAnonymous, resp.Metadata, nil
}

func (w *webhookFns) HandleMessage(ctx context.Context, auth types.WebsocketAuth, inputType string, data []byte) (string, []byte, error) {
	req := handleMessageReq{Auth: toAuthDTO(auth), InputType: inputType, Data: data}
	switch w.cfg.HandleMessageMode {
	case serverconfig.HandleMsgModeForward:
		w.async.Emit(webhook.EventMessageReceived, req)
		return "", nil, nil
	case serverconfig.HandleMsgModeDisabled:
		return "", nil, nil
	default: // sync
		var resp handleMessageResp
		if err := w.sync.Call(ctx, webhook.EventHandleMessage, req, w.cfg.HandleMessageTimeout, &resp); err != nil {
			// surfaces as an error frame to the client — must be sanitized.
			return "", nil, sanitizeCallError(webhook.EventHandleMessage, "upstream error", err)
		}
		return resp.OutputType, resp.Data, nil
	}
}

func (w *webhookFns) OnNewConnection(ctx context.Context, auth types.WebsocketAuth) error {
	// Fail closed: only a 2xx from the backend admits the connection.
	if err := w.sync.Call(ctx, webhook.EventOnNewConnection, authEvent{Auth: toAuthDTO(auth)}, w.cfg.SyncTimeout, nil); err != nil {
		return sanitizeCallError(webhook.EventOnNewConnection, "connection rejected", err)
	}
	w.async.Emit(webhook.EventOnNewConnectionEstablished, authEvent{Auth: toAuthDTO(auth)})
	return nil
}

func (w *webhookFns) OnCloseConnection(_ context.Context, auth types.WebsocketAuth) {
	w.async.Emit(webhook.EventOnCloseConnection, authEvent{Auth: toAuthDTO(auth)})
}

func (w *webhookFns) OnReadError(_ context.Context, auth types.WebsocketAuth, err error) {
	w.async.Emit(webhook.EventOnReadError, errorEvent{Auth: toAuthDTO(auth), Error: err.Error()})
}

func (w *webhookFns) OnWriteError(_ context.Context, auth types.WebsocketAuth, err error) {
	w.async.Emit(webhook.EventOnWriteError, errorEvent{Auth: toAuthDTO(auth), Error: err.Error()})
}
