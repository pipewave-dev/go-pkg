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
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (observer.Observability, error) {
	cfg := do.MustInvoke[configprovider.ConfigStore](i)
	otelProvider := do.MustInvoke[otelprv.OtelProvider](i)
	slogIns := do.MustInvoke[*slog.Logger](i)

	env := cfg.Env()
	logLevel := slog.Level(env.Otel.LogLevel)
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
		SlogLevel: logLevel,
	})

	return obsIns, nil
}
