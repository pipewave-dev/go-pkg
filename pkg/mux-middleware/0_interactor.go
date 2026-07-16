package muxmiddleware

import (
	"context"
	"net/http"
)

type (
	HandlerFn  = func(http.ResponseWriter, *http.Request)
	Middleware = func(http.Handler) http.Handler
)

type MiddlewareProvider interface {
	RequestID(
		callbackFn func(context.Context, string) context.Context,
	) Middleware
	JSONLogFmt(
		logFn func(context.Context, LogStruct), // if nil, will log via slog with "muxmw-access-log" msg
	) Middleware
	PanicRecover() Middleware
	Otel() Middleware
	Skip(mw Middleware, skipCondFn func(*http.Request) bool) Middleware
}

type MWConfig struct {
	IgnoreAccessLogPath []string
	TraceIDHeader       string
	// RedactQueryParams lists query string keys (e.g. "tk") whose values are replaced with
	// "REDACTED" before the request URL is written to the access log, so secrets passed
	// via query string never end up in logs.
	RedactQueryParams []string
}

type middlewareProvider struct {
	config *MWConfig
}

func NewMiddlewareProvider(
	config *MWConfig,
) MiddlewareProvider {
	return &middlewareProvider{
		config,
	}
}
