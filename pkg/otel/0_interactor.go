package otel

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

type OtelProvider interface {
	Shutdown(context.Context) error

	GetOtel(c context.Context, spanName string) (ctx context.Context, span trace.Span)

	// Propagation returns the carrier for the context.
	Propagation(ctx context.Context) (carrier []byte)

	// Extract extracts the context from the carrier.
	Extract(ctx context.Context, carrier []byte) (context.Context, error)
}

type OtelConfig struct {
	AppName string

	ExporterType      string  // one of: discard	stdout	file	otlp-grpc 	otlp-http
	FilePath          *string // require if ExporterType == file
	CollectorEndpoint *string // require if ExporterType == otlp-grpc or otlp-http
	Insecure          bool    // use insecure connection (no TLS) for OTLP

	// Set attr to span
	ExtractAttr func(context.Context) map[string]string
}

type otelClient struct {
	config     *OtelConfig
	shutdownFn func(context.Context) error
}

func NewOtelProvider(
	config *OtelConfig,
) OtelProvider {
	ins := &otelClient{
		config: config,
	}

	shutdown, err := ins.setup(context.Background())
	if err != nil {
		panic(err)
	}

	ins.shutdownFn = shutdown

	return ins
}
