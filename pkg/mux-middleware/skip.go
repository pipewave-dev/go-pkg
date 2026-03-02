package muxmiddleware

import "net/http"

func (m *middlewareProvider) Skip(mw Middleware, skipCondFn func(*http.Request) bool) Middleware {
	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skipCondFn(r) {
				next.ServeHTTP(w, r)
				return
			}
			wrapped.ServeHTTP(w, r)
		})
	}
}
