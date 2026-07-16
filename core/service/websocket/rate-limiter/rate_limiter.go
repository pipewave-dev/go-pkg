package ratelimiter

import (
	"sync"

	wsSv "github.com/pipewave-dev/go-pkg/core/service/websocket"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/samber/do/v2"

	voAuth "github.com/pipewave-dev/go-pkg/core/domain/value-object/auth"
	"golang.org/x/time/rate"
)

func NewDI(i do.Injector) (wsSv.RateLimiter, error) {
	c := do.MustInvoke[configprovider.ConfigStore](i)
	instance := &rateLimiter{
		c: c,

		userLimiter:      make(map[string]*rate.Limiter),
		userSessions:     make(map[string]map[string]struct{}),
		anonymousLimiter: make(map[string]*rate.Limiter),
	}

	return instance, nil
}

type rateLimiter struct {
	c configprovider.ConfigStore

	userLimiter      map[string]*rate.Limiter       // key = userID
	userSessions     map[string]map[string]struct{} // key = userID -> set of instanceID
	anonymousLimiter map[string]*rate.Limiter       // key = instanceID
	mu               sync.RWMutex
}

// New tạo (hoặc trả về đã có) limiter cho auth. Idempotent theo (UserID, InstanceID):
// gọi nhiều lần cho cùng một connection (do race giữa Get() fallback và onNew) sẽ không
// tạo/đếm session trùng lặp.
func (r *rateLimiter) New(auth voAuth.WebsocketAuth) *rate.Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()

	if auth.IsAnonymous() {
		if lim, ok := r.anonymousLimiter[auth.InstanceID]; ok {
			return lim
		}
		anonymousRate := r.c.Env().RateLimiter.AnonymousRate
		anonymousBurst := r.c.Env().RateLimiter.AnonymousBurst

		lim := rate.NewLimiter(rate.Limit(anonymousRate), anonymousBurst)
		r.anonymousLimiter[auth.InstanceID] = lim
		return lim
	}

	lim, ok := r.userLimiter[auth.UserID]
	if !ok {
		userRate := r.c.Env().RateLimiter.UserRate
		userBurst := r.c.Env().RateLimiter.UserBurst

		lim = rate.NewLimiter(rate.Limit(userRate), userBurst)
		r.userLimiter[auth.UserID] = lim
	}

	sessions, ok := r.userSessions[auth.UserID]
	if !ok {
		sessions = make(map[string]struct{})
		r.userSessions[auth.UserID] = sessions
	}
	sessions[auth.InstanceID] = struct{}{}

	return lim
}

func (r *rateLimiter) Remove(auth voAuth.WebsocketAuth) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if auth.IsAnonymous() {
		delete(r.anonymousLimiter, auth.InstanceID)
		return
	}

	sessions, ok := r.userSessions[auth.UserID]
	if !ok {
		return
	}
	delete(sessions, auth.InstanceID)
	if len(sessions) == 0 {
		delete(r.userSessions, auth.UserID)
		delete(r.userLimiter, auth.UserID)
	}
}

func (r *rateLimiter) Get(auth voAuth.WebsocketAuth) *rate.Limiter {
	r.mu.RLock()
	lim, ok := r.lookup(auth) // đọc thuần, không gọi New
	r.mu.RUnlock()
	if ok {
		return lim
	}
	return r.New(auth) // New tự lấy write-lock, không còn giữ RLock
}

func (r *rateLimiter) lookup(auth voAuth.WebsocketAuth) (lim *rate.Limiter, ok bool) {
	if auth.IsAnonymous() {
		lim, ok = r.anonymousLimiter[auth.InstanceID]
	} else {
		lim, ok = r.userLimiter[auth.UserID]
	}
	return
}
