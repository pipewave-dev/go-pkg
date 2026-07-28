package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Pinger chủ động POST một `ping` event tới backend callback endpoint để kiểm
// tra endpoint còn sống. Dùng cho boot-check (Ping) và runtime health (Run).
type Pinger struct {
	sender    *Sender
	timeout   time.Duration
	threshold int
}

func NewPinger(sender *Sender, timeout time.Duration, threshold int) *Pinger {
	if threshold < 1 {
		threshold = 1
	}
	return &Pinger{sender: sender, timeout: timeout, threshold: threshold}
}

// Ping gửi một ping và trả nil nếu backend trả 2xx.
func (p *Pinger) Ping(ctx context.Context) error {
	status, _, err := p.sender.Post(ctx, EventPing, NewCallbackID(), struct{}{}, p.timeout)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("webhook: ping returned status %d", status)
	}
	return nil
}

// Run ping theo interval tới khi ctx done. Chuỗi fail liên tiếp đạt threshold
// gọi onUnhealthy (mỗi lần đạt/vượt ngưỡng); một 2xx reset streak và gọi
// onHealthy.
func (p *Pinger) Run(ctx context.Context, interval time.Duration, onHealthy, onUnhealthy func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	streak := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.Ping(ctx); err != nil {
				streak++
				slog.Warn("[webhook] ping failed", "streak", streak, "error", err)
				if streak >= p.threshold {
					onUnhealthy()
				}
			} else {
				if streak > 0 {
					slog.Info("[webhook] ping recovered", "prev_streak", streak)
				}
				streak = 0
				onHealthy()
			}
		}
	}
}
