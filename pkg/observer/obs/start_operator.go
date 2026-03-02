package obs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/pkg/observer/fieldvalue"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// StartOperation - Enhanced with better resource management
func (o *observability) StartOperation(
	ctx context.Context,
	name string,
	opts ...observer.OperationOption,
) (context.Context, observer.Operation) {
	cf := observer.DefaultConfig()

	for _, opt := range opts {
		opt.Apply(cf)
	}

	if o.cf.GetTraceIdFn != nil {
		observer.WithOpFields(
			fieldvalue.String(
				"traceId", o.cf.GetTraceIdFn(ctx))).
			Apply(cf)
	}

	if o.cf.GetAuthStringFn != nil {
		observer.WithOpFields(
			fieldvalue.String(
				"auth", o.cf.GetAuthStringFn(ctx))).
			Apply(cf)
	}

	// Start span if tracing enabled
	var span trace.Span
	if o.cf.OtelTrace != nil {
		var attrs = []attribute.KeyValue{
			// attribute.String("operation.name", name),
			// attribute.String("service.name", o.cf.ServiceName),
			attribute.String("service.version", o.cf.ServiceVersion),
		}
		attrs = append(attrs, fieldsToAttribute(cf.Fields())...)

		if len(cf.Carrier()) != 0 {
			var err error
			ctx, err = o.cf.OtelTrace.Extract(ctx, cf.Carrier())
			attrs = append(attrs, attribute.String(
				"trace.type", "extracted"))
			if err != nil {
				attrs = append(attrs, attribute.String(
					"trace.extract.error", err.Error()))
			}
		}

		ctx, span = otel.Tracer(o.cf.ServiceName).Start(ctx, name,
			trace.WithAttributes(
				attrs...,
			),
			trace.WithSpanKind(cf.SpanKind()),
		)
	}

	// Create operation logger with fields
	var operationLogger *slog.Logger
	if o.cf.Slogger != nil {
		operationLogger = o.cf.Slogger
	} else {
		operationLogger = slog.Default()
	}
	if len(cf.Fields()) > 0 {
		operationLogger = operationLogger.With(fieldsToSlogAttrs(cf.Fields())...)
	}

	operationLogger.InfoContext(ctx, fmt.Sprintf("%s: Started", name))
	return ctx, &operation{
		ctx:       &ctx,
		span:      span,
		slogLevel: o.cf.SlogLevel,
		otelTrace: o.cf.OtelTrace,
		name:      name,
		isError:   false,
		slogger:   operationLogger,
		startTime: time.Now(),
		fields:    cf.Fields(),
	}
}
