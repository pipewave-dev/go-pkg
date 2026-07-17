package webhook

import (
	"encoding/json"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// SignatureHeader carries the base64 Ed25519 signature over the raw body.
const SignatureHeader = "X-Pipewave-Signature"

// Event type discriminants carried in Meta.EventType. Class-1 (sync,
// response expected): inspect_token, handle_message, on_new_connection.
// Class-2 (async, fire-and-forget with retry): the rest.
const (
	EventInspectToken               = "inspect_token"
	EventHandleMessage              = "handle_message"
	EventOnNewConnection            = "on_new_connection"
	EventOnNewConnectionEstablished = "on_new_connection_established"
	EventOnCloseConnection          = "on_close_connection"
	EventOnReadError                = "on_read_error"
	EventOnWriteError               = "on_write_error"
	EventMessageReceived            = "message_received"
)

type Meta struct {
	SentAt     int64  `json:"sent_at"` // unix milliseconds
	CallbackID string `json:"id"`      // idempotency key; retries reuse it
	EventType  string `json:"event_type"`
}

type Body struct {
	Data json.RawMessage `json:"data"`
	Meta Meta            `json:"meta"`
}

func NewCallbackID() string {
	return "cb_" + gonanoid.Must()
}
