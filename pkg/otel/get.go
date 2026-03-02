package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (s *otelClient) GetOtel(c context.Context, spanName string) (ctx context.Context, span trace.Span) {
	ctx, span = otel.Tracer(s.config.AppName).Start(c, spanName)
	attrMap := s.config.ExtractAttr(ctx)
	for k, v := range attrMap {
		span.SetAttributes(attribute.String(k, v))
	}
	return ctx, span
}
