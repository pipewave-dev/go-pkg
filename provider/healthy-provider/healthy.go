package healthyprovider

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (Healthy, error) {
	cfg := do.MustInvoke[configprovider.ConfigStore](i)
	// Default is unhealthy
	initHealthy := &atomic.Bool{} // Default is false

	return &healthy{
		history:   make([]*healthyHistory, 0),
		isHealthy: initHealthy,
		cfg:       cfg,
	}, nil
}

type Healthy = *healthy

type healthy struct {
	isHealthy *atomic.Bool
	history   []*healthyHistory
	cfg       configprovider.ConfigStore
}

type healthyHistory struct {
	Timestamp int64
	IsHealthy bool
	Reason    string
}

// IsHealthy returns the current health status
func (h *healthy) IsHealthy() bool {
	return h.isHealthy.Load()
}

// History returns the health history
func (h *healthy) History() []*healthyHistory {
	return h.history
}

// SetHealthy marks the service as healthy
func (h *healthy) SetHealthy(reason string) {
	if !h.isHealthy.Load() {
		h.isHealthy.Store(true)
		h.history = append(h.history, &healthyHistory{
			Timestamp: time.Now().UnixMilli(),
			IsHealthy: true,
			Reason:    reason,
		})
	}
}

// SetUnhealthy marks the service as unhealthy and sends notification
func (h *healthy) SetUnhealthy(reason string) {
	if h.isHealthy.Load() {
		h.isHealthy.Store(false)
		h.history = append(h.history, &healthyHistory{
			Timestamp: time.Now().UnixMilli(),
			IsHealthy: false,
			Reason:    reason,
		})

		env := h.cfg.Env()
		msg := fmt.Sprintf("Pod[%s] container-id[%s] is unhealthy. Reason: %s", env.Info.PodName, env.Info.ContainerID, reason)
		slog.WarnContext(context.Background(), msg)
	}
}
