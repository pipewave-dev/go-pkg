package otel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
)

func (s *otelClient) setup(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up trace provider.
	tracerProvider, err := s.newTraceProvider(ctx)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up propagator.
	otel.SetTextMapPropagator(newPropagator())

	// // Set up meter provider.
	//TODO Update soon...
	// meterProvider, err := s.newMeterProvider(ctx)
	// if err != nil {
	// 	handleErr(err)
	// 	return
	// }
	// shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	// otel.SetMeterProvider(meterProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func (s *otelClient) newTraceProvider(ctx context.Context) (*trace.TracerProvider, error) {

	var traceExporter trace.SpanExporter
	switch s.config.ExporterType {
	case "discard":
		var err error
		traceExporter, err = stdouttrace.New(
			stdouttrace.WithWriter(io.Discard))
		if err != nil {
			return nil, fmt.Errorf("failed to create discard trace exporter: %w", err)
		}
	case "stdout":
		stdoutExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		traceExporter = stdoutExporter
		if err != nil {
			return nil, fmt.Errorf("failed to create trace exporter: %w", err)
		}
	case "file":
		f, err := os.OpenFile(*s.config.FilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		traceExporter, err = stdouttrace.New(
			stdouttrace.WithWriter(f),
			stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("failed to create trace exporter: %w", err)
		}
	case "otlp-grpc":
		otelAgentAddr := *s.config.CollectorEndpoint
		conn, err := createGrpcConn(otelAgentAddr, s.config.Insecure)
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
		}
		traceExporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithGRPCConn(conn))
		if err != nil {
			return nil, fmt.Errorf("failed to create trace exporter: %w", err)
		}
	case "otlp-http":
		otelAgentAddr := *s.config.CollectorEndpoint
		var err error

		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(otelAgentAddr),
		}
		if s.config.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}

		traceExporter, err = otlptracehttp.New(ctx, opts...)

		if err != nil {
			return nil, fmt.Errorf("failed to create trace exporter: %w", err)
		}
	}

	traceResource := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(s.config.AppName),
		// semconv.ServiceVersionKey.String("global.Version"),
		// attribute.String("env", "global.Env"),
	)
	traceProvider := trace.NewTracerProvider(
		// trace.WithSampler(trace.AlwaysSample()),
		trace.WithResource(traceResource),
		trace.WithBatcher(traceExporter,
			trace.WithBatchTimeout(15*time.Second)),
	)

	return traceProvider, nil
}

// func (s *otelClient) newMeterProvider(ctx context.Context) (*metric.MeterProvider, error) {
// 	metricExporter, err := stdoutmetric.New()
// 	if err != nil {
// 		return nil, err
// 	}

// 	meterProvider := metric.NewMeterProvider(
// 		metric.WithReader(metric.NewPeriodicReader(metricExporter,
// 			// Default is 1m. Set to 3s for demonstrative purposes.
// 			metric.WithInterval(1*time.Minute))),
// 	)
// 	return meterProvider, nil
// }

func createGrpcConn(otelAgentAddr string, isInsecure bool) (*grpc.ClientConn, error) {
	dialOps := []grpc.DialOption{}
	if isInsecure {
		dialOps = append(dialOps, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Configure connection parameters to avoid premature timeout.
	dialOps = append(dialOps,
		// Disable idle timeout so the connection stays READY.
		grpc.WithIdleTimeout(0),

		// Configure backoff for faster reconnects.
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  300 * time.Millisecond, // Initial retry delay
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   3 * time.Second, // Max delay (instead of the default 120s)
			},
			MinConnectTimeout: 5 * time.Second, // Timeout per connection attempt
		}),
	)

	conn, err := grpc.NewClient(otelAgentAddr, dialOps...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
	}

	conn.Connect()
	return conn, nil
}
