package muxmiddleware

import (
	"context"
	"net/http"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// RequestID generates or reads a trace ID from the request header,
// sets it in the response header, and injects it into the request context.
func (mw *middlewareProvider) RequestID(callbackFn func(context.Context, string) context.Context) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := r.Header.Get(mw.config.TraceIDHeader)
			if rid == "" {
				rid = gonanoid.Must()
				r.Header.Set(mw.config.TraceIDHeader, rid)
			}
			w.Header().Set(mw.config.TraceIDHeader, rid)

			ctx := r.Context()
			if callbackFn != nil {
				ctx = callbackFn(ctx, rid)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
