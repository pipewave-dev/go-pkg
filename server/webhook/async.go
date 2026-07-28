package webhook

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// DefaultBackoff is the retry schedule for async events; the last value
// repeats for attempts beyond its length.
var DefaultBackoff = []time.Duration{
	time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute,
}

const (
	asyncQueueSize   = 1024
	asyncPostTimeout = 10 * time.Second
)

type asyncJob struct {
	eventType  string
	callbackID string
	data       any
	attempt    int // delivery attempts already made
}

// AsyncDispatcher delivers Class-2 events at-least-once with in-memory
// retry. Events are dropped (with a warning log) when the queue is full or
// when retryMax is exhausted. Events are also dropped on shutdown/crash —
// accepted for v1 — but in that case (and in a narrow shutdown-vs-retry-timer
// race, where select chooses arbitrarily between the closed-queue and
// enqueue-into-queue cases) the drop may occur without a warning log.
type AsyncDispatcher struct {
	sender   *Sender
	backoff  []time.Duration
	retryMax int

	queue     chan asyncJob
	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func NewAsyncDispatcher(sender *Sender, retryMax int, backoff []time.Duration) *AsyncDispatcher {
	if len(backoff) == 0 {
		backoff = DefaultBackoff
	}
	d := &AsyncDispatcher{
		sender:   sender,
		backoff:  backoff,
		retryMax: retryMax,
		queue:    make(chan asyncJob, asyncQueueSize),
		closed:   make(chan struct{}),
	}
	d.wg.Add(1)
	go d.loop()
	return d
}

// Emit enqueues an event without blocking the caller (WS hot paths call
// this). A full queue drops the event (with a warning log). Calling Emit
// during or after Shutdown is not guaranteed to log a drop — the event may
// enqueue into a queue that will never be drained again.
func (d *AsyncDispatcher) Emit(eventType string, data any) {
	job := asyncJob{eventType: eventType, callbackID: NewCallbackID(), data: data}
	select {
	case d.queue <- job:
	default:
		slog.Warn("[webhook] async queue full, dropping event", "event_type", eventType)
	}
}

// Shutdown stops accepting retries and drains queued events best-effort
// until ctx expires.
func (d *AsyncDispatcher) Shutdown(ctx context.Context) {
	d.closeOnce.Do(func() { close(d.closed) })
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (d *AsyncDispatcher) loop() {
	defer d.wg.Done()
	for {
		select {
		case job := <-d.queue:
			d.deliver(job)
		case <-d.closed:
			for {
				select {
				case job := <-d.queue:
					d.deliver(job)
				default:
					return
				}
			}
		}
	}
}

func (d *AsyncDispatcher) deliver(job asyncJob) {
	status, _, err := d.sender.PostWithMode(context.Background(), job.eventType, job.callbackID, job.data, asyncPostTimeout, ModeAsync)
	job.attempt++
	if err == nil && status >= 200 && status < 300 {
		return
	}

	if job.attempt >= d.retryMax {
		slog.Warn("[webhook] dropping event after max retries",
			"event_type", job.eventType, "callback_id", job.callbackID, "attempts", job.attempt, "last_status", status, "error", err)
		if d.sender.obs != nil {
			d.sender.obs.ObserveDropped(job.eventType)
		}
		return
	}

	if d.sender.obs != nil {
		d.sender.obs.ObserveRetry(job.eventType, ModeAsync)
	}

	delay := d.backoff[min(job.attempt-1, len(d.backoff)-1)]
	time.AfterFunc(delay, func() {
		select {
		case <-d.closed:
		case d.queue <- job:
		default:
			slog.Warn("[webhook] async queue full, dropping retried event",
				"event_type", job.eventType, "callback_id", job.callbackID)
			if d.sender.obs != nil {
				d.sender.obs.ObserveDropped(job.eventType)
			}
		}
	})
}
