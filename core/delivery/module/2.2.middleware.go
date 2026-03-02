package moduledelivery

import (
	"context"
	"net/http"
	"regexp"
	"slices"

	"github.com/pipewave-dev/go-pkg/shared/actx"
)

func (m *moduleDelivery) AllMiddilewares(h http.Handler) http.Handler {
	mw := []Middleware{
		m.mw.JSONLogFmt(nil),
		m.mw.RequestID(func(ctx context.Context, rId string) context.Context {
			aCtx := actx.From(ctx)
			aCtx.SetTraceID(rId)
			return ctx
		}),
		m.mw.PanicRecover(),
		// m.mw.InjectCmdQueue(),
	}
	if m.c.Env().Otel.Enabled {
		mw = append(mw, m.mw.Otel())
	}
	if m.c.Env().Cors.Enabled {
		mw = append(mw, m.corsMiddleware)
	}
	return chain(h, mw...)
}

func (m *moduleDelivery) WsMiddlewares(h http.Handler) http.Handler {
	return chain(h,
		m.mw.RequestID(func(ctx context.Context, rId string) context.Context {
			aCtx := actx.From(ctx)
			aCtx.SetTraceID(rId)
			return ctx
		}),
		m.corsMiddleware,
	)
}

func chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// TODO: for debug only, remove later
func (m *moduleDelivery) corsMiddleware(next http.Handler) http.Handler {
	config := m.c.Env().Cors
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isAllowedOrigin(origin, config.ExactlyOrigins, config.RegexOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Pipewave-ID")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isAllowedOrigin(origin string, exactlyOrigins, regexOrigins []string) bool {
	if slices.Contains(exactlyOrigins, origin) {
		return true
	}
	for _, pattern := range regexOrigins {
		matched, err := regexp.MatchString(pattern, origin)
		if err == nil && matched {
			return true
		}
	}
	return false
}
