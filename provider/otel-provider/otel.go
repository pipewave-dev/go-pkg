package otelprovider

import (
	"context"

	"github.com/pipewave-dev/go-pkg/global/constants"
	otelprv "github.com/pipewave-dev/go-pkg/pkg/otel"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	"github.com/pipewave-dev/go-pkg/shared/actx"
	"github.com/samber/lo"
)

// New creates a new OpenTelemetry provider with injected config.
// This replaces the singleton pattern in singleton/otel with dependency injection.
func New(cfg configprovider.ConfigStore, cleanupTask fncollector.CleanupTask) otelprv.OtelProvider {
	env := cfg.Env()

	otelIns := otelprv.NewOtelProvider(&otelprv.OtelConfig{
		AppName:           constants.AppNameShort,
		ExporterType:      env.Otel.ExporterType,
		CollectorEndpoint: lo.ToPtr(env.Otel.CollectorEndpoint),
		Insecure:          env.Otel.CollectorInsecure,
		ExtractAttr: func(ctx context.Context) map[string]string {
			aCtx := actx.From(ctx)
			rid := aCtx.GetTraceID()
			return map[string]string{
				"traceID": rid,
			}
		},
	})

	// Register cleanup task
	cleanupTask.RegTask(func() {
		_ = otelIns.Shutdown(context.Background())
	}, fncollector.FnPriorityNormal)

	return otelIns
}
