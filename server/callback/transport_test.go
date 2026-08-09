package callback_test

import (
	"context"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/callback"
	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

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
