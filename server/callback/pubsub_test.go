package callback_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/pipewave-dev/go-pkg/server/callback"
	"github.com/pipewave-dev/go-pkg/server/webhook"
	"github.com/stretchr/testify/require"
)

type capturedMsg struct {
	subject string
	payload []byte
}

// fakePub ghi lại message. blockUntil, nếu khác nil, chặn Publish cho tới
// khi nó được đóng — mô phỏng broker chết.
type fakePub struct {
	mu         sync.Mutex
	got        []capturedMsg
	blockUntil chan struct{}
}

func (f *fakePub) Publish(_ context.Context, subject string, payload []byte) error {
	if f.blockUntil != nil {
		<-f.blockUntil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, capturedMsg{subject: subject, payload: payload})
	return nil
}

func (f *fakePub) Healthcheck() error { return nil }

func (f *fakePub) messages() []capturedMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]capturedMsg(nil), f.got...)
}

func TestPubsubTransport_SubjectAndEnvelope(t *testing.T) {
	pub := &fakePub{}
	tr := callback.NewPubsubTransport(pub, "pipewave.events")

	tr.Emit(webhook.EventOnCloseConnection, map[string]string{"user_id": "u1"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tr.Shutdown(ctx)

	msgs := pub.messages()
	require.Len(t, msgs, 1)
	require.Equal(t, "pipewave.events.on_close_connection", msgs[0].subject)

	var env webhook.Body
	require.NoError(t, json.Unmarshal(msgs[0].payload, &env))
	require.Equal(t, webhook.EventOnCloseConnection, env.Meta.EventType)
	require.NotEmpty(t, env.Meta.CallbackID)
	require.NotZero(t, env.Meta.SentAt)
	require.JSONEq(t, `{"user_id":"u1"}`, string(env.Data))
}

// Test QUAN TRỌNG NHẤT: broker chết không được làm nghẽn WS hot path.
func TestPubsubTransport_EmitNeverBlocks(t *testing.T) {
	pub := &fakePub{blockUntil: make(chan struct{})}
	tr := callback.NewPubsubTransport(pub, "pipewave.events")

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Nhiều hơn sức chứa buffer — vẫn phải trả về ngay.
		for i := 0; i < 5000; i++ {
			tr.Emit(webhook.EventOnReadError, map[string]string{"n": "x"})
		}
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Emit blocked while broker was stalled")
	}

	close(pub.blockUntil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tr.Shutdown(ctx)
}
