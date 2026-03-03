package cacheprovider

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/cache"
)

func CacheThis[ResultT any, Err error](ctx context.Context,
	store cache.CacheProvider,
	ttl time.Duration,
	cacheKeyFn string,
	fetchFn func(context.Context) (ResultT, Err),
) (
	ResultT, Err,
) {
	c := &cacheThis[ResultT, Err]{
		cacheKey: cacheKeyFn,
		ttl:      ttl,
		fetchFn:  fetchFn,
		store:    store,
	}
	return c.do(ctx)
}

type cacheThis[ResultT any, Err error] struct {
	store    cache.CacheProvider
	cacheKey string
	ttl      time.Duration
	fetchFn  func(context.Context) (ResultT, Err)
}

func (ct *cacheThis[ResultT, Err]) do(ctx context.Context) (ResultT, Err) {
	val := new(ResultT)
	nilErr := new(Err)
	if found := ct.store.Get(ctx, ct.cacheKey, val); found {
		return *val, *nilErr
	}

	fresh, err := ct.fetchFn(ctx)
	// NOTE: Err is often an interface alias (e.g. aerror.AError) and can be nil.
	// Calling methods on a nil underlying pointer/interface will panic, so guard first.
	if any(err) != nil {
		var empty ResultT
		return empty, err
	}
	go ct.store.Set(context.WithoutCancel(ctx), ct.cacheKey, &fresh, ct.ttl)
	return fresh, *nilErr
}
