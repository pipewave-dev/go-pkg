package webhook

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// HealthMonitor hội tụ tín hiệu sức khỏe backend từ nhiều nguồn (pinger,
// breaker watcher). onUnhealthy được gọi ĐÚNG MỘT LẦN ở lần chuyển
// healthy→unhealthy đầu tiên. Package này không biết callback đó làm gì —
// wiring ở main.go quyết định (graceful shutdown hoặc log-only).
type HealthMonitor struct {
	mu          sync.Mutex
	healthy     bool
	fired       bool
	onUnhealthy func()
}

func NewHealthMonitor(onUnhealthy func()) *HealthMonitor {
	return &HealthMonitor{healthy: true, onUnhealthy: onUnhealthy}
}

func (m *HealthMonitor) SetHealthy() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = true
}

func (m *HealthMonitor) SetUnhealthy(reason string) {
	m.mu.Lock()
	fire := !m.fired
	m.fired = true
	m.healthy = false
	m.mu.Unlock()
	if fire {
		slog.Error("[webhook] backend marked unhealthy", "reason", reason)
		if m.onUnhealthy != nil {
			m.onUnhealthy()
		}
	}
}

func (m *HealthMonitor) IsHealthy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.healthy
}

// WatchBreakerOpen báo unhealthy nếu breaker mở liên tục >= maxOpen. Ticker
// chạy ở min(maxOpen, 5s) để phát hiện kịp thời. Block tới ctx done.
func WatchBreakerOpen(ctx context.Context, b *CircuitBreaker, maxOpen time.Duration, m *HealthMonitor) {
	tick := maxOpen
	if tick > 5*time.Second || tick <= 0 {
		tick = 5 * time.Second
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if at, ok := b.OpenSince(); ok && time.Since(at) >= maxOpen {
				m.SetUnhealthy("circuit breaker open >= " + maxOpen.String())
			}
		}
	}
}
