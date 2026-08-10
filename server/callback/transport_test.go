package callback_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/callback"
	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

// stubTransport là một AsyncTransport tối giản để test HealthyFunc:
// Healthcheck trả đúng err được nạp vào.
type stubTransport struct{ err error }

func (s *stubTransport) Emit(string, any)         {}
func (s *stubTransport) Healthcheck() error       { return s.err }
func (s *stubTransport) Shutdown(context.Context) {}

// HealthyFunc phải phản ánh sức khoẻ của transport: transport hỏng
// (vd NATS mất kết nối) ⇒ không healthy, để /healthz trả 503.
func TestHealthyFunc_ReflectsTransportHealth(t *testing.T) {
	healthy := callback.HealthyFunc(&stubTransport{err: nil})
	require.True(t, healthy(), "transport khoẻ ⇒ healthy")

	unhealthy := callback.HealthyFunc(&stubTransport{err: errors.New("not connected")})
	require.False(t, unhealthy(), "transport hỏng ⇒ unhealthy")
}

// nil transport không được panic — nó nghĩa là "không có ràng buộc".
func TestHealthyFunc_NilTransportIsHealthy(t *testing.T) {
	require.True(t, callback.HealthyFunc(nil)())
}

// AsyncDispatcher phải thoả AsyncTransport để webhook vẫn là default
// mà không cần đổi gì ở fns.
func TestAsyncDispatcherSatisfiesAsyncTransport(t *testing.T) {
	d := webhook.NewAsyncDispatcher(
		webhook.NewSender("http://127.0.0.1:1", nil),
		1,
		[]time.Duration{time.Millisecond},
	)
	var tr callback.AsyncTransport = d
	require.NoError(t, tr.Healthcheck())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tr.Shutdown(ctx)
}
