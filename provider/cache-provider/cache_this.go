package cacheprovider

import (
	"context"
	"log/slog"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/cache"
	"golang.org/x/sync/singleflight"
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

	// fetchGroup dedupes concurrent CacheThis misses for the same cacheKey so only
	// one fetchFn call runs at a time per key, instead of every concurrent miss
	// hitting the backing source (cache stampede).
	fetchGroup singleflight.Group
}

// fetchResult carries fetchFn's outcome through singleflight.Group.Do, which
// only supports a single `any` return value.
type fetchResult[ResultT any, Err error] struct {
	val ResultT
	err Err
}

func (ct *cacheThis[ResultT, Err]) do(ctx context.Context) (ResultT, Err) {
	val := new(ResultT)
	nilErr := new(Err)
	if found := ct.store.Get(ctx, ct.cacheKey, val); found {
		return *val, *nilErr
	}

	v, _, _ := ct.fetchGroup.Do(ct.cacheKey, func() (any, error) {
		fresh, err := ct.fetchFn(ctx)
		// NOTE: Err is often an interface alias (e.g. aerror.AError) and can be nil.
		// Calling methods on a nil underlying pointer/interface will panic, so guard first.
		if any(err) != nil {
			var empty ResultT
			return fetchResult[ResultT, Err]{val: empty, err: err}, nil
		}

		if setable := ct.store.Set(context.WithoutCancel(ctx), ct.cacheKey, &fresh, ct.ttl); !setable {
			slog.WarnContext(ctx, "CacheThis: failed to set cache value", slog.String("cacheKey", ct.cacheKey))
		}
		return fetchResult[ResultT, Err]{val: fresh}, nil
	})

	res := v.(fetchResult[ResultT, Err])
	if any(res.err) != nil {
		var empty ResultT
		return empty, res.err
	}
	return res.val, *nilErr
}
