package msghub_test

import (
	"context"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	msghub "github.com/pipewave-dev/go-pkg/core/service/websocket/msg-hub"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

// fakeRepo is an in-memory implementation of repository.PendingMessageRepo.
type fakeRepo struct {
	data map[string][][]byte // key: userID+":"+instanceID
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{data: make(map[string][][]byte)}
}

func repoKey(userID, instanceID string) string {
	return userID + ":" + instanceID
}

func (f *fakeRepo) Create(_ context.Context, userID, instanceID string, _ time.Time, message []byte) aerror.AError {
	key := repoKey(userID, instanceID)
	f.data[key] = append(f.data[key], message)
	return nil
}

func (f *fakeRepo) GetAll(_ context.Context, userID, instanceID string) ([][]byte, aerror.AError) {
	key := repoKey(userID, instanceID)
	msgs := f.data[key]
	if len(msgs) == 0 {
		return nil, nil
	}
	// return a copy so caller cannot mutate internal state
	result := make([][]byte, len(msgs))
	copy(result, msgs)
	return result, nil
}

func (f *fakeRepo) DeleteAll(_ context.Context, userID, instanceID string) aerror.AError {
	key := repoKey(userID, instanceID)
	delete(f.data, key)
	return nil
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestRegisterAndIsRegistered(t *testing.T) {
	svc := msghub.New(newFakeRepo(), 5*time.Second)

	svc.Register("user1", "inst1", func() {})

	assert.True(t, svc.IsRegistered("user1", "inst1"), "registered session should be found")
	assert.False(t, svc.IsRegistered("user1", "unknown"), "unknown instance should not be found")
	assert.False(t, svc.IsRegistered("unknown", "inst1"), "unknown user should not be found")
}

func TestDeregisterCancelsTimer(t *testing.T) {
	svc := msghub.New(newFakeRepo(), 10*time.Second) // long TTL

	var fired atomic.Bool
	svc.Register("user1", "inst1", func() { fired.Store(true) })

	svc.Deregister("user1", "inst1")

	assert.False(t, svc.IsRegistered("user1", "inst1"), "session should be removed after deregister")

	// give the goroutine a moment to potentially (wrongly) fire
	time.Sleep(50 * time.Millisecond)
	assert.False(t, fired.Load(), "onExpired must NOT fire after Deregister")
}

func TestExpiredTimerFires(t *testing.T) {
	svc := msghub.New(newFakeRepo(), 20*time.Millisecond)

	fired := make(chan struct{}, 1)
	svc.Register("user1", "inst1", func() { fired <- struct{}{} })

	select {
	case <-fired:
		// good – timer fired
	case <-time.After(200 * time.Millisecond):
		t.Fatal("onExpired did not fire within 200ms")
	}

	assert.False(t, svc.IsRegistered("user1", "inst1"), "session should be removed after TTL expiry")
}

func TestGetSessions(t *testing.T) {
	svc := msghub.New(newFakeRepo(), 10*time.Second)

	svc.Register("userA", "inst1", func() {})
	svc.Register("userA", "inst2", func() {})
	svc.Register("userA", "inst3", func() {})
	svc.Register("userB", "inst1", func() {})

	sessionsA := svc.GetSessions("userA")
	require.Len(t, sessionsA, 3, "userA should have 3 sessions")
	sort.Strings(sessionsA)
	assert.Equal(t, []string{"inst1", "inst2", "inst3"}, sessionsA)

	sessionsB := svc.GetSessions("userB")
	require.Len(t, sessionsB, 1, "userB should have 1 session")
	assert.Equal(t, []string{"inst1"}, sessionsB)

	assert.Nil(t, svc.GetSessions("userC"), "unknown user should return nil")
}

func TestSaveAndConsume(t *testing.T) {
	repo := newFakeRepo()
	svc := msghub.New(repo, 10*time.Second)
	ctx := context.Background()

	svc.Register("user1", "inst1", func() {})

	msg1 := []byte("hello")
	msg2 := []byte("world")

	require.NoError(t, svc.Save(ctx, "user1", "inst1", msg1))
	require.NoError(t, svc.Save(ctx, "user1", "inst1", msg2))

	msgs, err := svc.Consume(ctx, "user1", "inst1")
	require.NoError(t, err)
	require.Len(t, msgs, 2, "Consume should return both saved messages")
	assert.Equal(t, msg1, msgs[0])
	assert.Equal(t, msg2, msgs[1])

	// repo should be empty after successful consume
	remaining, aErr := repo.GetAll(ctx, "user1", "inst1")
	assert.Nil(t, aErr)
	assert.Empty(t, remaining, "repo should be empty after Consume")
}

func TestConsumeEmptyIsNoError(t *testing.T) {
	svc := msghub.New(newFakeRepo(), 10*time.Second)
	ctx := context.Background()

	svc.Register("user1", "inst1", func() {})

	msgs, err := svc.Consume(ctx, "user1", "inst1")
	assert.NoError(t, err, "Consume on empty session should not return an error")
	assert.Empty(t, msgs, "Consume on empty session should return empty/nil slice")
}
