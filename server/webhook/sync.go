package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrCircuitOpen is returned without touching the backend while the
// breaker is open. Callers treat it like any other sync-call failure
// (fail closed / error frame).
var ErrCircuitOpen = errors.New("webhook: circuit breaker is open")

// CallError is a non-2xx answer from the callback receiver. 4xx bodies are
// application-level answers (e.g. rejected connection), not infrastructure
// failures.
type CallError struct {
	Status int
	Body   []byte
}

func (e *CallError) Error() string {
	return fmt.Sprintf("webhook: callback returned status %d: %s", e.Status, e.Body)
}

// CircuitBreaker opens after `threshold` consecutive infrastructure
// failures and lets traffic through again once `cooldown` has elapsed
// (all requests probe; the first success closes it).
type CircuitBreaker struct {
	mu        sync.Mutex
	threshold int
	cooldown  time.Duration
	failures  int
	openedAt  time.Time
	now       func() time.Time
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{threshold: threshold, cooldown: cooldown, now: time.Now}
}

func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures < b.threshold {
		return true
	}
	return b.now().Sub(b.openedAt) >= b.cooldown
}

func (b *CircuitBreaker) Record(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if success {
		b.failures = 0
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.openedAt = b.now()
	}
}

// OpenSince trả thời điểm breaker chuyển open nếu HIỆN ĐANG open (chưa được
// một probe thành công đóng lại), cùng ok=true. Không mở → (time.Time{}, false).
func (b *CircuitBreaker) OpenSince() (time.Time, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures < b.threshold {
		return time.Time{}, false
	}
	return b.openedAt, true
}

// SyncCaller performs Class-1 (request/response) callback invocations.
type SyncCaller struct {
	sender   *Sender
	breaker  *CircuitBreaker
	retryMax int
	backoff  time.Duration
}

func NewSyncCaller(sender *Sender, breaker *CircuitBreaker, retryMax int, backoff time.Duration) *SyncCaller {
	if retryMax < 1 {
		retryMax = 1
	}
	return &SyncCaller{sender: sender, breaker: breaker, retryMax: retryMax, backoff: backoff}
}

// Call posts the event and decodes a 2xx JSON response into out (out may be
// nil). Non-2xx returns *CallError. Only transport errors and 5xx are
// recorded as breaker failures AND retried (up to retryMax attempts, reusing one
// callbackID so receivers dedupe). 4xx and circuit-open short-circuit without
// retry. Retries stop early if ctx is done.
func (c *SyncCaller) Call(ctx context.Context, eventType string, data any, timeout time.Duration, out any) error {
	callbackID := NewCallbackID()
	var lastErr error
	for attempt := 0; attempt < c.retryMax; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(c.backoff):
			}
		}
		if !c.breaker.Allow() {
			return ErrCircuitOpen
		}
		status, body, err := c.sender.Post(ctx, eventType, callbackID, data, timeout)
		if err != nil {
			c.breaker.Record(false)
			lastErr = err
			continue // transport error → retry
		}
		if status < 200 || status >= 300 {
			c.breaker.Record(status < 500)
			if status < 500 {
				return &CallError{Status: status, Body: body} // 4xx: deliberate, no retry
			}
			lastErr = &CallError{Status: status, Body: body}
			continue // 5xx → retry
		}
		c.breaker.Record(true)
		if out != nil && len(body) > 0 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("webhook: decode %s response: %w", eventType, err)
			}
		}
		return nil
	}
	return lastErr
}
