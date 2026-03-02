package ratelimiter

import (
	"sync"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"golang.org/x/time/rate"
)

func New(c configprovider.ConfigStore) wsSv.RateLimiter {
	instance := &rateLimiter{
		c: c,

		userLimiter:      make(map[string]*rate.Limiter),
		userSessionCount: make(map[string]int),
		anonymousLimiter: make(map[string]*rate.Limiter),
	}

	return instance
}

type rateLimiter struct {
	c configprovider.ConfigStore

	userLimiter      map[string]*rate.Limiter // key = userID
	userSessionCount map[string]int           // key = userID
	anonymousLimiter map[string]*rate.Limiter // key = instanceID
	mu               sync.RWMutex
}

func (r *rateLimiter) New(auth voAuth.WebsocketAuth) *rate.Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()

	if auth.IsAnonymous() {
		anonymousRate := r.c.Env().RateLimiter.AnonymousRate
		anonymousBurst := r.c.Env().RateLimiter.AnonymousBurst

		r.anonymousLimiter[auth.InstanceID] = rate.NewLimiter(rate.Limit(anonymousRate), anonymousBurst)
		return r.anonymousLimiter[auth.InstanceID]
	} else {
		userRate := r.c.Env().RateLimiter.UserRate
		userBurst := r.c.Env().RateLimiter.UserBurst

		_, ok := r.userLimiter[auth.UserID]
		if !ok {
			r.userLimiter[auth.UserID] = rate.NewLimiter(rate.Limit(userRate), userBurst)
			r.userSessionCount[auth.UserID] = 1
		} else {
			r.userSessionCount[auth.UserID]++
		}
		return r.userLimiter[auth.UserID]
	}
}

func (r *rateLimiter) Remove(auth voAuth.WebsocketAuth) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if auth.IsAnonymous() {
		delete(r.anonymousLimiter, auth.InstanceID)
		return
	}
	r.userSessionCount[auth.UserID]--
	if r.userSessionCount[auth.UserID] == 0 {
		delete(r.userLimiter, auth.UserID)
		delete(r.userSessionCount, auth.UserID)
	}
}

func (r *rateLimiter) Get(auth voAuth.WebsocketAuth) *rate.Limiter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var (
		rate *rate.Limiter
		ok   bool
	)
	if auth.IsAnonymous() {
		rate, ok = r.anonymousLimiter[auth.InstanceID]
		if ok {
			return rate
		}
	}
	rate, ok = r.userLimiter[auth.UserID]
	if ok {
		return rate
	}
	return r.New(auth)
}
