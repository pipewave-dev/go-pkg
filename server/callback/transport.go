// Package callback định nghĩa transport contract cho việc đẩy callback
// events ra backend người dùng.
package callback

import "context"

// AsyncTransport delivers Class-2 (fire-and-forget) callback events.
//
// Emit MUST NOT block: nó được gọi từ WebSocket hot paths. Implementation
// phải buffer nội bộ và deliver trên goroutine riêng; khi buffer đầy thì
// drop kèm warning log, KHÔNG chặn caller.
type AsyncTransport interface {
	// Emit enqueues an event without blocking the caller.
	Emit(eventType string, data any)
	// Healthcheck reports transport health. Trả nil nghĩa là khoẻ.
	Healthcheck() error
	// Shutdown drains best-effort cho tới khi ctx hết hạn.
	Shutdown(ctx context.Context)
}
