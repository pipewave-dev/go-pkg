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

// HealthyFunc adapts a transport's Healthcheck into the func() bool shape
// that restapi.MuxConfig.ExtraHealthy expects, so /healthz phản ánh được
// sức khoẻ của callback transport.
//
// Ở webhook mode, AsyncDispatcher.Healthcheck luôn trả nil (mỗi lần deliver
// là một HTTP request riêng, sức khoẻ backend do Pinger theo dõi), nên kết
// quả luôn true và /healthz giữ nguyên hành vi cũ. Ở pubsub mode, một
// broker mất kết nối sẽ khiến /healthz trả 503.
//
// A nil transport means "no constraint from this source" → healthy.
func HealthyFunc(t AsyncTransport) func() bool {
	return func() bool {
		if t == nil {
			return true
		}
		return t.Healthcheck() == nil
	}
}
