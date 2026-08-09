package callback

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
)

// queueSize khớp với AsyncDispatcher để hành vi backpressure giống nhau.
const queueSize = 1024

// Publisher là bề mặt hẹp mà PubsubTransport cần từ một pubsub adapter.
//
// HỢP ĐỒNG: Publish trả nil ⟹ broker đã DURABLE-ACK message. Adapter
// không đảm bảo được điều này (vd Redis/Valkey pub/sub: at-most-once,
// không ack) KHÔNG đủ tư cách làm callback transport.
type Publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
	Healthcheck() error
}

// PubsubTransport đẩy Class-2 events lên pubsub broker.
//
// Emit không bao giờ block: event vào buffered channel, một goroutine
// riêng publish. Queue đầy ⟹ drop kèm warning, giống AsyncDispatcher.
// Không retry in-memory — broker lo durability.
type PubsubTransport struct {
	pub           Publisher
	subjectPrefix string

	queue     chan pubsubJob
	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

type pubsubJob struct {
	subject string
	payload []byte
}

var _ AsyncTransport = (*PubsubTransport)(nil)

// NewPubsubTransport starts a background delivery goroutine tied to the
// returned transport. Callers MUST call Shutdown to release it — there is
// no other way to stop the loop, so a PubsubTransport dropped without
// Shutdown leaks that goroutine for the life of the process.
func NewPubsubTransport(pub Publisher, subjectPrefix string) *PubsubTransport {
	t := &PubsubTransport{
		pub:           pub,
		subjectPrefix: subjectPrefix,
		queue:         make(chan pubsubJob, queueSize),
		closed:        make(chan struct{}),
	}
	t.wg.Add(1)
	go t.loop()
	return t
}

// Emit enqueues an event without blocking the caller (WS hot paths call
// this). A full queue drops the event (with a warning log). Calling Emit
// during or after Shutdown is not guaranteed to log a drop — the event may
// enqueue into a queue that will never be drained again.
func (t *PubsubTransport) Emit(eventType string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		slog.Error("[callback/pubsub] marshal data", "event_type", eventType, "error", err)
		return
	}
	payload, err := json.Marshal(webhook.Body{
		Data: raw,
		Meta: webhook.Meta{
			SentAt:     nowUnixMilli(),
			CallbackID: webhook.NewCallbackID(),
			EventType:  eventType,
		},
	})
	if err != nil {
		slog.Error("[callback/pubsub] marshal envelope", "event_type", eventType, "error", err)
		return
	}

	select {
	case t.queue <- pubsubJob{subject: t.subjectPrefix + "." + eventType, payload: payload}:
	default:
		slog.Warn("[callback/pubsub] queue full, dropping event", "event_type", eventType)
	}
}

func (t *PubsubTransport) Healthcheck() error { return t.pub.Healthcheck() }

func (t *PubsubTransport) Shutdown(ctx context.Context) {
	t.closeOnce.Do(func() { close(t.closed) })
	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (t *PubsubTransport) loop() {
	defer t.wg.Done()
	for {
		select {
		case job := <-t.queue:
			t.deliver(job)
		case <-t.closed:
			for {
				select {
				case job := <-t.queue:
					t.deliver(job)
				default:
					return
				}
			}
		}
	}
}

func (t *PubsubTransport) deliver(job pubsubJob) {
	if err := t.pub.Publish(context.Background(), job.subject, job.payload); err != nil {
		slog.Warn("[callback/pubsub] publish failed", "subject", job.subject, "error", err)
	}
}

func nowUnixMilli() int64 { return time.Now().UnixMilli() }
