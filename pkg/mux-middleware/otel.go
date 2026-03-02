package muxmiddleware

import (
	"net/http"

	"github.com/samber/lo"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

/*
# Otel

Readmore at [Opentelemetry]

[Opentelemetry]: https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
*/
func (mw *middlewareProvider) Otel() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// otelHandler is created once per middleware setup, not per request.
		otelHandler := otelhttp.NewHandler(next, "")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if lo.Contains(mw.config.IgnoreAccessLogPath, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			otelHandler.ServeHTTP(w, r)
		})
	}
}
