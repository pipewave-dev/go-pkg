package moduledelivery

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/vmihailenco/msgpack/v5"
)

type (
	HandlerFn  = http.HandlerFunc
	Middleware = func(http.Handler) http.Handler
)

func handler[In, Out any](handler func(context.Context, In) (Out, aerror.AError)) HandlerFn {
	return func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/msgpack") || strings.Contains(contentType, "application/x-msgpack") {
			handlerMsgpack(handler)(w, r)
		} else {
			handlerJson(handler)(w, r)
		}
	}
}

func handlerJson[In, Out any](handler func(context.Context, In) (Out, aerror.AError)) HandlerFn {
	return func(w http.ResponseWriter, r *http.Request) {
		var req In
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, aErr := handler(r.Context(), req)
		if aErr != nil {
			http.Error(w, aErr.Error(), aErr.ErrorCode().HttpCode())
			return
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handlerMsgpack[In, Out any](handler func(context.Context, In) (Out, aerror.AError)) HandlerFn {
	return func(w http.ResponseWriter, r *http.Request) {
		var req In
		if err := msgpack.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, aErr := handler(r.Context(), req)
		if aErr != nil {
			http.Error(w, aErr.Error(), aErr.ErrorCode().HttpCode())
			return
		}
		if err := msgpack.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
