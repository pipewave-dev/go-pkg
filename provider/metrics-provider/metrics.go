// Package metricsprovider owns the process-level metrics pipeline for the
// pipewave container: a Prometheus exporter, the global MeterProvider, and a
// dedicated HTTP listener serving /metrics.
//
// Installing the global MeterProvider is correct HERE because the container
// owns the process. pkg/metrics (used by the embeddable library) only ever
// reads the global provider, so a Go host embedding pipewave keeps control of
// its own registry.
package metricsprovider

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/pipewave-dev/go-pkg/export/types"
	"github.com/pipewave-dev/go-pkg/global/constants"
	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/samber/do/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Provider owns the metrics exporter, MeterProvider and listener.
type Provider struct {
	cfg      *types.MetricsT
	metrics  *metrics.PipewaveMetrics
	mp       *sdkmetric.MeterProvider
	handler  http.Handler
	srv      *http.Server
	listener net.Listener
}

// NewDI builds the provider from the injector and registers a shutdown task.
func NewDI(i do.Injector) (*Provider, error) {
	cfg := do.MustInvoke[configprovider.ConfigStore](i)
	cleanupTask := do.MustInvoke[fncollector.CleanupTask](i)

	env := cfg.Env()
	p, err := newProvider(env.Metrics, env.Info.ContainerID)
	if err != nil {
		return nil, err
	}

	cleanupTask.RegTask(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.Shutdown(ctx); err != nil {
			slog.Warn("metrics: shutdown failed", slog.Any("error", err))
		}
	}, fncollector.FnPriorityNormal)

	return p, nil
}

// NewStandalone builds a provider outside the DI graph, for the container's
// main() and for tests.
func NewStandalone(cfg *types.MetricsT, containerID string) (*Provider, error) {
	return newProvider(cfg, containerID)
}

func newProvider(cfg *types.MetricsT, containerID string) (*Provider, error) {
	if cfg == nil {
		cfg = &types.MetricsT{}
	}
	p := &Provider{cfg: cfg}

	if !cfg.Enabled {
		// Do not touch the global provider. metrics.New falls back to the
		// no-op API, so every Record* call is free.
		p.metrics = metrics.New(metrics.Config{})
		return p, nil
	}

	exporter, err := prometheus.New()
	if err != nil {
		// Metrics must never stop the server from booting.
		slog.Error("metrics: prometheus exporter init failed; metrics disabled",
			slog.Any("error", err))
		p.cfg = &types.MetricsT{}
		p.metrics = metrics.New(metrics.Config{})
		return p, nil
	}

	p.mp = sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(p.mp)

	p.metrics = metrics.New(metrics.Config{
		MsgTypeAllowlist: cfg.MsgTypeAllowlist,
		Version:          constants.Version,
		ContainerID:      containerID,
	})

	mux := http.NewServeMux()
	mux.Handle(cfg.Path, promhttp.Handler())
	p.handler = mux

	return p, nil
}

// Metrics returns the instrument set. Never nil.
func (p *Provider) Metrics() *metrics.PipewaveMetrics { return p.metrics }

// Handler returns the /metrics handler, or nil when metrics are disabled.
func (p *Provider) Handler() http.Handler { return p.handler }

// ListenAndServe starts the metrics listener and blocks until shutdown.
// Returns nil immediately when metrics are disabled.
//
// Callers should run this in a goroutine and log (not fatal) on error: a
// metrics listener that fails to bind must not take the server down.
func (p *Provider) ListenAndServe() error {
	if p.handler == nil {
		return nil
	}
	addr := ":" + strconv.Itoa(p.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	p.listener = ln
	p.srv = &http.Server{Handler: p.handler, ReadHeaderTimeout: 5 * time.Second}
	slog.Info("metrics: listening", slog.String("addr", ln.Addr().String()),
		slog.String("path", p.cfg.Path))
	if err := p.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the listener and flushes the MeterProvider.
func (p *Provider) Shutdown(ctx context.Context) error {
	var firstErr error
	if p.srv != nil {
		if err := p.srv.Shutdown(ctx); err != nil {
			firstErr = err
		}
	}
	if p.mp != nil {
		if err := p.mp.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
