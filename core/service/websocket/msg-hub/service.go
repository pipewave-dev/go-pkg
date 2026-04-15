package msghub

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	repo "github.com/pipewave-dev/go-pkg/core/repository"
	workerpool "github.com/pipewave-dev/go-pkg/pkg/worker-pool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	fncollector "github.com/pipewave-dev/go-pkg/provider/fn-collector"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (MessageHubSvc, error) {
	c := do.MustInvoke[configprovider.ConfigStore](i)
	allRepo := do.MustInvoke[repo.AllRepository](i)
	cleanupTask := do.MustInvoke[fncollector.CleanupTask](i)
	wp := do.MustInvoke[*workerpool.WorkerPool](i)

	cfg := c.Env().ActiveConnection
	ins := &msgHubSvc{
		registry: make(map[string]map[string]entry),
		repo:     allRepo.PendingMessage(),
		ttl:      cfg.HeartbeatCutoff + time.Minute, // ensure pending messages live at least until the next expected heartbeat
		wp:       wp,
	}

	cleanupTask.RegTask(ins.Shutdown, fncollector.FnPriorityNormal)
	return ins, nil
}

type entry struct {
	cancel context.CancelFunc
	gen    uint64
}

const shutdownReconnectGracePeriod = 1 * time.Second

type msgHubSvc struct {
	mu       sync.RWMutex
	registry map[string]map[string]entry // userID -> instanceID -> entry
	repo     repo.PendingMessageRepo
	ttl      time.Duration
	genSeq   atomic.Uint64

	wp *workerpool.WorkerPool
}

func New(
	c configprovider.ConfigStore,
	pendingRepo repo.PendingMessageRepo,
	cleanupTask fncollector.CleanupTask,
	wp *workerpool.WorkerPool,
) MessageHubSvc {
	cfg := c.Env().ActiveConnection
	ins := &msgHubSvc{
		registry: make(map[string]map[string]entry),
		repo:     pendingRepo,
		ttl:      cfg.HeartbeatCutoff + time.Minute, // ensure pending messages live at least until the next expected heartbeat
		wp:       wp,
	}

	cleanupTask.RegTask(ins.Shutdown, fncollector.FnPriorityNormal)
	return ins
}

func (s *msgHubSvc) Register(userID, instanceID string, onExpired func()) {
	s.registerWithTTL(userID, instanceID, s.ttl, onExpired)
}

func (s *msgHubSvc) registerWithTTL(userID, instanceID string, ttl time.Duration, onExpired func()) {
	ctx, cancel := context.WithCancel(context.Background())
	gen := s.genSeq.Add(1)

	s.mu.Lock()
	if s.registry[userID] == nil {
		s.registry[userID] = make(map[string]entry)
	}
	if prev, ok := s.registry[userID][instanceID]; ok {
		prev.cancel() // cancel any stale registration
	}
	s.registry[userID][instanceID] = entry{cancel: cancel, gen: gen}
	s.mu.Unlock()

	s.wp.Submit(func() {
		timer := time.NewTimer(ttl)
		defer timer.Stop()
		select {
		case <-timer.C:
			s.mu.Lock()
			if m, ok := s.registry[userID]; ok {
				if e, ok2 := m[instanceID]; ok2 && e.gen == gen {
					delete(m, instanceID)
					if len(m) == 0 {
						delete(s.registry, userID)
					}
				}
			}
			s.mu.Unlock()
			onExpired()
		case <-ctx.Done():
		}
	})
}

func (s *msgHubSvc) Deregister(userID, instanceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.registry[userID]; ok {
		if e, ok2 := m[instanceID]; ok2 {
			e.cancel()
			delete(m, instanceID)
		}
		if len(m) == 0 {
			delete(s.registry, userID)
		}
	}
}

func (s *msgHubSvc) IsRegistered(userID, instanceID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m, ok := s.registry[userID]; ok {
		_, ok2 := m[instanceID]
		return ok2
	}
	return false
}

func (s *msgHubSvc) GetSessions(userID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.registry[userID]
	if !ok {
		return nil
	}
	sessions := make([]string, 0, len(m))
	for instanceID := range m {
		sessions = append(sessions, instanceID)
	}
	return sessions
}

func (s *msgHubSvc) Save(ctx context.Context, userID, instanceID string, wrappedMsg []byte) error {
	// sendAt uses wall-clock time for ordering within the repo; this is intentional.
	aErr := s.repo.Create(ctx, userID, instanceID, time.Now(), wrappedMsg)
	if aErr != nil {
		slog.ErrorContext(ctx, "MessageHubSvc.Save: failed",
			slog.String("userID", userID),
			slog.String("instanceID", instanceID),
			slog.Any("error", aErr))
		return aErr
	}
	return nil
}

func (s *msgHubSvc) Consume(ctx context.Context, userID, instanceID string) ([][]byte, error) {
	msgs, aErr := s.repo.GetAll(ctx, userID, instanceID)
	if aErr != nil {
		slog.ErrorContext(ctx, "MessageHubSvc.Consume: GetAll failed",
			slog.String("userID", userID),
			slog.String("instanceID", instanceID),
			slog.Any("error", aErr))
		return nil, aErr
	}
	if len(msgs) == 0 {
		return nil, nil
	}
	s.DeleteAllPendingMessage(ctx, userID, instanceID)

	return msgs, nil
}

func (s *msgHubSvc) DeleteAllPendingMessage(ctx context.Context, userID, instanceID string) {
	if delErr := s.repo.DeleteAll(ctx, userID, instanceID); delErr != nil {
		slog.ErrorContext(ctx, "MessageHubSvc.DeleteAllPendingMessage: DeleteAll failed — messages may re-deliver on next reconnect",
			slog.String("userID", userID),
			slog.String("instanceID", instanceID),
			slog.Any("error", delErr))
	}
}

func (s *msgHubSvc) Shutdown() {
	time.Sleep(shutdownReconnectGracePeriod)

	type canceledSession struct {
		userID     string
		instanceID string
		cancel     context.CancelFunc
	}

	s.mu.Lock()
	sessions := make([]canceledSession, 0)
	for userID, instances := range s.registry {
		for instanceID, e := range instances {
			sessions = append(sessions, canceledSession{
				userID:     userID,
				instanceID: instanceID,
				cancel:     e.cancel,
			})
		}
	}
	s.registry = make(map[string]map[string]entry)
	s.mu.Unlock()

	for _, session := range sessions {
		session.cancel()
	}

	if len(sessions) == 0 {
		return
	}

	attrs := make([]any, 0, len(sessions)*2+1)
	attrs = append(attrs, slog.Int("cancelledTimers", len(sessions)))
	for _, session := range sessions {
		attrs = append(attrs,
			slog.String("userID", session.userID),
			slog.String("instanceID", session.instanceID),
		)
	}
	slog.Warn("MessageHubSvc.Shutdown: cancelled active temp-disconnect timers before expiry; pending messages remain until reconnect or separate cleanup", attrs...)
}
