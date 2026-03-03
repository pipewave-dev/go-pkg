package clientmsghandler

import (
	"sync"
	"time"

	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
)

const msgDeduplicateTTL = 2 * time.Minute

// msgDeduplicator tracks recently seen message IDs to prevent duplicate processing.
// A benign race may allow two concurrent goroutines to both return false for the
// same key — acceptable given the small probability window.
type msgDeduplicator struct {
	entries sync.Map
	ttl     time.Duration
}

func newMsgDeduplicator(intervalTask fncollector.IntervalTask) *msgDeduplicator {
	i := &msgDeduplicator{ttl: msgDeduplicateTTL}
	intervalTask.RegTask(i.clearExpiredEntries, fncollector.FnPriorityNormal)
	return i
}

// isDuplicate returns true if the key was already seen within the TTL window.
// If not a duplicate, records the key and returns false.
func (d *msgDeduplicator) isDuplicate(key string) bool {
	now := time.Now()
	if v, ok := d.entries.Load(key); ok {
		if now.Sub(v.(time.Time)) < d.ttl {
			return true
		}
	}
	d.entries.Store(key, now)
	return false
}

func (d *msgDeduplicator) clearExpiredEntries() {
	now := time.Now()
	d.entries.Range(func(key, value any) bool {
		if now.Sub(value.(time.Time)) >= d.ttl {
			d.entries.Delete(key)
		}
		return true
	})
}
