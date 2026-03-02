package otel

import (
	"context"
	"time"

	"github.com/samber/lo"
)

func (s *otelClient) Shutdown(ctx context.Context) error {
	_, _, err := lo.AttemptWithDelay(5, 20*time.Second, func(i int, duration time.Duration) error {
		return s.shutdownFn(ctx)
	})
	return err
}
