package callback

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pipewave-dev/go-pkg/server/webhook"
)

// queueSize khớp với AsyncDispatcher để hành vi backpressure giống nhau.
const queueSize = 1024

// publishTimeout bounds each Publish call. nats.go applies a 5s default API
// timeout internally when no deadline is set on ctx; this constant makes
// that previously-implicit library default explicit in our own code.
const publishTimeout = 5 * time.Second

// dropLogInterval rate-limits the queue-full warning log. The metric (via
// CallObserver.ObserveDropped) still counts every drop exactly — only the
// log line is throttled, to survive a broker outage under a high-rate
// WebSocket workload without flooding the log pipeline.
const dropLogInterval = 5 * time.Second

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
	obs           webhook.CallObserver

	queue     chan pubsubJob
	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup

	// lastDropLogNano is the unix-nano timestamp of the last queue-full
	// warning log, used to rate-limit that log line. Accessed only via
	// atomics — no lock, so Emit's hot path stays allocation- and lock-free.
	lastDropLogNano atomic.Int64
}

type pubsubJob struct {
	subject   string
	eventType string
	payload   []byte
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

// SetObserver attaches an observer for callback metrics. Safe to leave
// unset — a nil observer disables observation, matching webhook.Sender's
// convention. Not safe to call concurrently with Emit/deliver; wire it once
// at startup before traffic starts, same as Sender.SetObserver.
func (t *PubsubTransport) SetObserver(obs webhook.CallObserver) { t.obs = obs }

// Emit enqueues an event without blocking the caller (WS hot paths call
// this). A full queue drops the event: the metric (if an observer is set)
// counts every drop, but the warning log is rate-limited (see
// dropLogInterval) — under a broker outage, logging one line per dropped
// event would flood the log pipeline while the metric alone stays exact.
// Calling Emit during or after Shutdown is not guaranteed to log a drop —
// the event may enqueue into a queue that will never be drained again.
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
	case t.queue <- pubsubJob{subject: t.subjectPrefix + "." + eventType, eventType: eventType, payload: payload}:
	default:
		if t.obs != nil {
			t.obs.ObserveDropped(eventType)
		}
		t.logDropRateLimited(eventType)
	}
}

// logDropRateLimited emits the queue-full warning at most once per
// dropLogInterval, using a lock-free CAS on the last-logged timestamp so
// Emit's hot path never blocks on a mutex.
func (t *PubsubTransport) logDropRateLimited(eventType string) {
	now := time.Now().UnixNano()
	last := t.lastDropLogNano.Load()
	if now-last < int64(dropLogInterval) {
		return
	}
	if t.lastDropLogNano.CompareAndSwap(last, now) {
		slog.Warn("[callback/pubsub] queue full, dropping event (log rate-limited)",
			"event_type", eventType, "log_interval", dropLogInterval)
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
	// nats.go defaults to a 5s API timeout when ctx has no deadline; make
	// that explicit rather than relying on the library's implicit default.
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()

	start := time.Now()
	err := t.pub.Publish(ctx, job.subject, job.payload)
	if t.obs != nil {
		// statusCode 0: non-HTTP transport, "never got a response" fits.
		t.obs.ObserveCall(job.eventType, webhook.ModePubsub, time.Since(start), 0, err)
	}
	if err != nil {
		slog.Warn("[callback/pubsub] publish failed", "subject", job.subject, "error", err)
	}
}

func nowUnixMilli() int64 { return time.Now().UnixMilli() }
