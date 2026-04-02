package observerprovider

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pipewave-dev/go-pkg/global/constants"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/pkg/observer/obs"
	otelprv "github.com/pipewave-dev/go-pkg/pkg/otel"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/actx"
)

// New creates a new observability provider with injected config and dependencies.
// This replaces the singleton pattern in singleton/observer with dependency injection.
func New(
	cfg configprovider.ConfigStore,
	otelProvider otelprv.OtelProvider,
	slogIns *slog.Logger,
) observer.Observability {
	env := cfg.Env()

	obsIns := obs.NewObservability(&obs.ObservabilityConfig{
		ServiceName:    constants.AppNameShort,
		ServiceVersion: env.Version,
		Environment:    env.Env,
		GetTraceIdFn: func(ctx context.Context) string {
			traceId := actx.From(ctx).GetTraceID()
			return traceId
		},
		GetAuthStringFn: func(ctx context.Context) string {
			auth := actx.From(ctx).GetWebsocketAuth()
			if auth.InstanceID == "" {
				return "" // No auth info available
			}
			if auth.IsAnonymous() {
				return fmt.Sprintf("anon[%s]", auth.InstanceID)
			}
			return fmt.Sprintf("%s@%s", auth.UserID, auth.InstanceID)
		},
		OtelTrace: otelProvider,
		Slogger:   slogIns,
		SlogLevel: slog.LevelInfo,
	})

	return obsIns
}
