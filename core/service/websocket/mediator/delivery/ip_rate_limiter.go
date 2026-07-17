package delivery

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipLimiterEntryTTL: entries idle longer than this are pruned opportunistically
// so the map doesn't grow unbounded as distinct client IPs come and go.
const ipLimiterEntryTTL = 10 * time.Minute

// ipRateLimiter throttles requests per client IP. Used at /issue-tmp-token to
// bound how fast a single IP can mint fresh anonymous InstanceIDs, independent
// of the per-instance limiter applied once a session exists (rate-limiter/rate_limiter.go).
type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiterEntry
	rateVal  rate.Limit
	burst    int
}

type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newIPRateLimiter(r rate.Limit, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		limiters: make(map[string]*ipLimiterEntry),
		rateVal:  r,
		burst:    burst,
	}
}

func (l *ipRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	entry, ok := l.limiters[ip]
	if !ok {
		entry = &ipLimiterEntry{limiter: rate.NewLimiter(l.rateVal, l.burst)}
		l.limiters[ip] = entry
	}
	entry.lastSeen = now

	// Opportunistic cleanup instead of a background goroutine: bound map growth
	// without needing lifecycle management for a ticker.
	if len(l.limiters)%256 == 0 {
		for k, v := range l.limiters {
			if now.Sub(v.lastSeen) > ipLimiterEntryTTL {
				delete(l.limiters, k)
			}
		}
	}

	return entry.limiter.Allow()
}

// clientIP extracts the request's client IP, preferring the configured proxy
// header (e.g. X-Forwarded-For, first entry) and falling back to RemoteAddr.
func clientIP(r *http.Request, ipHeader string) string {
	if ipHeader != "" {
		if v := r.Header.Get(ipHeader); v != "" {
			first, _, _ := strings.Cut(v, ",")
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
