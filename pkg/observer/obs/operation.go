package obs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer/fieldvalue"

	otelWrapper "github.com/pipewave-dev/go-pkg/pkg/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Operation represents a complete business operation with observability
type operation struct {
	ctx       *context.Context
	span      trace.Span
	slogLevel slog.Level
	slogger   *slog.Logger
	otelTrace otelWrapper.OtelProvider

	isError bool

	name      string
	startTime time.Time
	fields    []fieldvalue.Field
}

func (op *operation) Propagation() (carrier []byte) {
	if op.span != nil {
		return op.otelTrace.Propagation(*op.ctx)
	}
	return nil
}

func (op *operation) AddFields(field fieldvalue.Field, other ...fieldvalue.Field) {
	fields := append([]fieldvalue.Field{field}, other...)
	op.fields = append(op.fields, fields...)
	op.slogger = op.slogger.With(fieldsToSlogAttrs(fields)...)

	if op.span != nil {
		op.span.SetAttributes(fieldsToAttribute(fields)...)
	}
}

func (op *operation) AddStep(msg string, fields ...fieldvalue.Field) {
	if op.span != nil {
		op.span.AddEvent(msg)
	}
	op.slogger.Log(*op.ctx, op.slogLevel, fmt.Sprintf("%s: %s", op.name, msg))
}

func (op *operation) RecordError(err error) {
	op.isError = true
	if op.span != nil {
		op.span.RecordError(err)
		op.span.SetStatus(codes.Error, err.Error())
	}
}

func (op *operation) Success() {
	duration := time.Since(op.startTime)
	if op.span != nil {
		op.span.SetAttributes(attribute.String("operation.duration", duration.String()))
		op.span.SetStatus(codes.Ok, "")
		defer op.span.End()
	}

	op.slogger.Log(*op.ctx, op.slogLevel, fmt.Sprintf("%s: Succeeded", op.name),
		slog.Duration("duration", duration),
	)
}

func (op *operation) Failure(err error) {
	duration := time.Since(op.startTime)
	if op.span != nil {
		op.span.SetAttributes(attribute.String("operation.duration", duration.String()))
		op.RecordError(err)
		defer op.span.End()
	}
	op.slogger.Log(*op.ctx, op.slogLevel, fmt.Sprintf("%s: Failed", op.name),
		slog.Duration("duration", duration),
	)
}

func (op *operation) Finish(err error) {
	if err != nil {
		op.Failure(err)
	} else {
		op.Success()
	}
}
