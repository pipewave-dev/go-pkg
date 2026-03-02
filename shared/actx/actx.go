package actx

import (
	"context"
	"sync"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
)

type alterData struct {
	m sync.Mutex

	traceId       string
	parentTraceId []string
	auth          voAuth.Auth

	fromBroadcast bool

	userIp    string
	userAgent string
}
type aContext struct {
	context.Context
	data *alterData
}

type AContext = *aContext

func From(ctx context.Context) AContext {
	if ctx == nil {
		ctx = context.Background()
	}

	aData, ok := ctx.Value(privKey).(*alterData)
	if ok {
		return &aContext{
			Context: ctx,
			data:    aData,
		}
	} else {
		newAData := alterData{
			m:      sync.Mutex{},
			userIp: "",
			auth:   voAuth.NoAuth(),
		}
		ctx = context.WithValue(ctx, privKey, &newAData)
		return &aContext{
			Context: ctx,
			data:    &newAData,
		}
	}
}

type aCtxKey int

const (
	_ aCtxKey = iota + 1
	privKey
)
