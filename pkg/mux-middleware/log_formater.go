package muxmiddleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/samber/lo"
)

// LogStruct - logger structure.
type LogStruct struct {
	IP        string `json:"ip"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	StartTime string `json:"start_time"`
	Duration  string `json:"duration"`
	Agent     string `json:"agent"`
	Status    int    `json:"status"`
}

// responseWriter wraps http.ResponseWriter to capture the written status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// JSONLogFmt prints JSON-formatted access logs to STDOUT via slog.
func (mw *middlewareProvider) JSONLogFmt(
	logFn func(context.Context, LogStruct),
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if lo.Contains(mw.config.IgnoreAccessLogPath, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			rw := newResponseWriter(w)
			t := time.Now()
			next.ServeHTTP(rw, r)

			entry := LogStruct{
				IP:        r.RemoteAddr,
				Method:    r.Method,
				URL:       redactQueryParams(r.RequestURI, mw.config.RedactQueryParams),
				StartTime: t.Format(time.RFC3339),
				Duration:  time.Since(t).String(),
				Agent:     r.UserAgent(),
				Status:    rw.statusCode,
			}

			ctx := r.Context()
			if logFn != nil {
				logFn(ctx, entry)
				return
			} else {
				defaultLogFn(ctx, entry)
			}
		})
	}
}

// redactQueryParams replaces the value of each key in redactKeys with "REDACTED" in
// rawURI's query string, so sensitive values (e.g. connection tokens) never reach logs.
func redactQueryParams(rawURI string, redactKeys []string) string {
	if len(redactKeys) == 0 {
		return rawURI
	}

	u, err := url.ParseRequestURI(rawURI)
	if err != nil {
		return rawURI
	}

	q := u.Query()
	redacted := false
	for _, key := range redactKeys {
		if q.Has(key) {
			q.Set(key, "REDACTED")
			redacted = true
		}
	}
	if !redacted {
		return rawURI
	}

	u.RawQuery = q.Encode()
	return u.RequestURI()
}

func defaultLogFn(ctx context.Context, entry LogStruct) {
	var level slog.Level
	if entry.Status >= http.StatusInternalServerError {
		level = slog.LevelError
	} else {
		level = slog.LevelInfo
	}

	slog.LogAttrs(ctx, level, "muxmw-access-log", slog.Any("http", entry))
}
