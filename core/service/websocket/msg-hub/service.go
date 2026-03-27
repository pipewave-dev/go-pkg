package msghub

import (
	"context"
	"log/slog"
	"sync"
	"time"

	repo "github.com/pipewave-dev/go-pkg/core/repository"
)

type msgHubSvc struct {
	mu       sync.RWMutex
	registry map[string]map[string]context.CancelFunc // userID -> instanceID -> cancelFn
	repo     repo.PendingMessageRepo
	ttl      time.Duration
}

func New(pendingRepo repo.PendingMessageRepo, ttl time.Duration) MessageHubSvc {
	return &msgHubSvc{
		registry: make(map[string]map[string]context.CancelFunc),
		repo:     pendingRepo,
		ttl:      ttl,
	}
}

func (s *msgHubSvc) Register(userID, instanceID string, onExpired func()) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.registry[userID] == nil {
		s.registry[userID] = make(map[string]context.CancelFunc)
	}
	if prev, ok := s.registry[userID][instanceID]; ok {
		prev() // cancel any stale registration
	}
	s.registry[userID][instanceID] = cancel
	s.mu.Unlock()

	go func() {
		select {
		case <-time.After(s.ttl):
			s.mu.Lock()
			if m, ok := s.registry[userID]; ok {
				delete(m, instanceID)
				if len(m) == 0 {
					delete(s.registry, userID)
				}
			}
			s.mu.Unlock()
			onExpired()
		case <-ctx.Done():
		}
	}()
}

func (s *msgHubSvc) Deregister(userID, instanceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.registry[userID]; ok {
		if cancel, ok2 := m[instanceID]; ok2 {
			cancel()
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
	if delErr := s.repo.DeleteAll(ctx, userID, instanceID); delErr != nil {
		slog.ErrorContext(ctx, "MessageHubSvc.Consume: DeleteAll failed — messages may re-deliver on next reconnect",
			slog.String("userID", userID),
			slog.String("instanceID", instanceID),
			slog.Any("error", delErr))
	}
	return msgs, nil
}
