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

// fakeObserver implements webhook.CallObserver and counts observations.
// Guarded by a mutex so concurrent Emit/deliver goroutines stay -race clean.
type fakeObserver struct {
	mu      sync.Mutex
	calls   int
	retries int
	dropped int
	drops   []string
}

func (o *fakeObserver) ObserveCall(_, _ string, _ time.Duration, _ int, _ error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls++
}

func (o *fakeObserver) ObserveRetry(_, _ string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.retries++
}

func (o *fakeObserver) ObserveDropped(eventType string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.dropped++
	o.drops = append(o.drops, eventType)
}

func (o *fakeObserver) droppedCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.dropped
}

func (o *fakeObserver) callCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

var _ webhook.CallObserver = (*fakeObserver)(nil)

// TestPubsubTransport_ObserverReceivesDrop proves that when the queue is
// full, Emit reports the drop through the observer (fix for the metrics
// blind-spot: pubsub mode must not bypass pipewave_callback_dropped_total).
func TestPubsubTransport_ObserverReceivesDrop(t *testing.T) {
	pub := &fakePub{blockUntil: make(chan struct{})}
	tr := callback.NewPubsubTransport(pub, "pipewave.events")
	obs := &fakeObserver{}
	tr.SetObserver(obs)

	// Publish blocks (broker down): the delivery goroutine picks up exactly
	// one job and stalls there, so the queue fills up after queueSize more
	// enqueues. Emit far more than the queue capacity to guarantee drops.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5000; i++ {
			tr.Emit(webhook.EventOnReadError, map[string]string{"n": "x"})
		}
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Emit blocked while broker was stalled")
	}

	require.Greater(t, obs.droppedCount(), 0, "expected at least one drop to be observed")
	for _, et := range obs.drops {
		require.Equal(t, webhook.EventOnReadError, et)
	}

	close(pub.blockUntil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tr.Shutdown(ctx)

	// The one job that made it through before the queue filled up (plus
	// whatever drained after unblocking) should be observed as a call.
	require.GreaterOrEqual(t, obs.callCount(), 1)
}

// TestPubsubTransport_NilObserverSafe proves Emit and deliver never panic
// when no observer has been wired — the documented default.
func TestPubsubTransport_NilObserverSafe(t *testing.T) {
	pub := &fakePub{}
	tr := callback.NewPubsubTransport(pub, "pipewave.events")
	// No SetObserver call: obs stays nil.

	require.NotPanics(t, func() {
		tr.Emit(webhook.EventOnCloseConnection, map[string]string{"user_id": "u1"})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NotPanics(t, func() {
		tr.Shutdown(ctx)
	})

	require.Len(t, pub.messages(), 1)
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
