package muxmiddleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// PanicRecover catches any panic from downstream handlers,
// logs the stack trace, and responds with 500 Internal Server Error.
func (mw *middlewareProvider) PanicRecover() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					mw.internalServerErrorResponseStackTraceHandler(w, r, rec)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func (mw *middlewareProvider) internalServerErrorResponseStackTraceHandler(w http.ResponseWriter, r *http.Request, e interface{}) {
	ctx := r.Context()
	reportBug := fmt.Sprintf("<<PANIC:%v>>\n%s", e, string(debug.Stack()))
	slog.ErrorContext(ctx, "panic", "stacktrace", reportBug)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": "golang panic",
	})
}
