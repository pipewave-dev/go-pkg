package clientmsghandler

import (
	"sync"
	"time"

	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
)

const heartbeatThrottleDuration = 10 * time.Second

// heartbeatThrottle limits DynamoDB writes to at most once per throttleDuration per key.
// A benign race may allow two concurrent goroutines to both return true for the
// same key — acceptable for heartbeat throttling (worst case: 1 extra write).
type heartbeatThrottle struct {
	entries sync.Map
	ttl     time.Duration
}

func newHeartbeatThrottle(intervalTask fncollector.IntervalTask) *heartbeatThrottle {
	i := &heartbeatThrottle{ttl: heartbeatThrottleDuration}
	intervalTask.RegTask(i.clearExpiredEntries, fncollector.FnPriorityNormal)
	return i
}

func (t *heartbeatThrottle) shouldUpdate(key string) bool {
	now := time.Now()
	if v, ok := t.entries.Load(key); ok {
		if now.Sub(v.(time.Time)) < t.ttl {
			return false
		}
	}
	t.entries.Store(key, now)
	return true
}

func (t *heartbeatThrottle) clearExpiredEntries() {
	now := time.Now()
	t.entries.Range(func(key, value any) bool {
		if now.Sub(value.(time.Time)) >= t.ttl {
			t.entries.Delete(key)
		}
		return true
	})
}
