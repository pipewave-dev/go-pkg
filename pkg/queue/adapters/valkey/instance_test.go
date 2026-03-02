package valkey

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcvalkey "github.com/testcontainers/testcontainers-go/modules/valkey"
)

// setupValkeyContainer starts a Valkey testcontainer and returns a configured adapter.
// The caller must call cleanup() when done.
func setupValkeyContainer(t *testing.T) (*valkeyAdapter, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := tcvalkey.Run(ctx, "valkey/valkey:8")
	require.NoError(t, err, "failed to start valkey container")

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)

	endpoint := fmt.Sprintf("%s:%s", host, port.Port())

	cfg := &Config{
		ValkeyEndpoint: endpoint,
		Password:       "",
		DB:             0,
		Prefix:         "test:",
	}

	adapter := New(cfg).(*valkeyAdapter)

	cleanup := func() {
		adapter.Close()
		_ = container.Terminate(ctx)
	}

	return adapter, cleanup
}

func TestHealthcheck(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	err := adapter.Healthcheck(ctx)
	assert.NoError(t, err, "healthcheck should succeed on a running valkey")
}

func TestPublishAndFetchOne(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "channel1"
	message := []byte("hello world")

	// Publish a message
	err := adapter.Publish(ctx, channel, message)
	require.NoError(t, err)

	// FetchOne should return the message
	result, err := adapter.FetchOne(ctx, channel)
	require.NoError(t, err)
	assert.Equal(t, message, result)
}

func TestFetchOne_EmptyQueue(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	result, err := adapter.FetchOne(ctx, "empty-channel")
	assert.NoError(t, err)
	assert.Nil(t, result, "FetchOne on empty queue should return nil")
}

func TestFetchOne_FIFO(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "fifo-channel"

	// Publish multiple messages
	for i := 0; i < 3; i++ {
		err := adapter.Publish(ctx, channel, []byte(fmt.Sprintf("msg-%d", i)))
		require.NoError(t, err)
	}

	// FetchOne should return messages in FIFO order (RPUSH + LPOP)
	for i := 0; i < 3; i++ {
		result, err := adapter.FetchOne(ctx, channel)
		require.NoError(t, err)
		assert.Equal(t, []byte(fmt.Sprintf("msg-%d", i)), result)
	}

	// Queue should now be empty
	result, err := adapter.FetchOne(ctx, channel)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestFetchMany(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "many-channel"

	// Publish 5 messages
	for i := 0; i < 5; i++ {
		err := adapter.Publish(ctx, channel, []byte(fmt.Sprintf("item-%d", i)))
		require.NoError(t, err)
	}

	// FetchMany with maxItem=3 should return first 3
	messages, err := adapter.FetchMany(ctx, channel, 3)
	require.NoError(t, err)
	require.Len(t, messages, 3)
	assert.Equal(t, []byte("item-0"), messages[0])
	assert.Equal(t, []byte("item-1"), messages[1])
	assert.Equal(t, []byte("item-2"), messages[2])

	// Remaining 2 messages should still be in queue
	messages, err = adapter.FetchMany(ctx, channel, 10)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, []byte("item-3"), messages[0])
	assert.Equal(t, []byte("item-4"), messages[1])
}

func TestFetchMany_EmptyQueue(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	messages, err := adapter.FetchMany(ctx, "empty-many", 5)
	assert.NoError(t, err)
	assert.Nil(t, messages, "FetchMany on empty queue should return nil")
}

func TestFetchMany_LessThanMaxItem(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "less-channel"

	// Publish 2 messages, fetch with maxItem=5
	for i := 0; i < 2; i++ {
		err := adapter.Publish(ctx, channel, []byte(fmt.Sprintf("x-%d", i)))
		require.NoError(t, err)
	}

	messages, err := adapter.FetchMany(ctx, channel, 5)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.Equal(t, []byte("x-0"), messages[0])
	assert.Equal(t, []byte("x-1"), messages[1])
}

func TestPublish_BinaryData(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "binary-channel"
	binaryMsg := []byte{0x00, 0x01, 0xFF, 0xFE, 0x80, 0x7F}

	err := adapter.Publish(ctx, channel, binaryMsg)
	require.NoError(t, err)

	result, err := adapter.FetchOne(ctx, channel)
	require.NoError(t, err)
	assert.Equal(t, binaryMsg, result)
}

func TestPrefix(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "prefix-test"
	msg := []byte("prefixed")

	err := adapter.Publish(ctx, channel, msg)
	require.NoError(t, err)

	// Verify the key uses the prefix by fetching via the adapter
	// (which also prepends the prefix)
	result, err := adapter.FetchOne(ctx, channel)
	require.NoError(t, err)
	assert.Equal(t, msg, result)

	// Different channel (without prefix match) should be empty
	result, err = adapter.FetchOne(ctx, "other-channel")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestMultipleChannels(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()

	// Publish to different channels
	err := adapter.Publish(ctx, "ch-a", []byte("msg-a"))
	require.NoError(t, err)
	err = adapter.Publish(ctx, "ch-b", []byte("msg-b"))
	require.NoError(t, err)

	// Fetch from each channel independently
	resultA, err := adapter.FetchOne(ctx, "ch-a")
	require.NoError(t, err)
	assert.Equal(t, []byte("msg-a"), resultA)

	resultB, err := adapter.FetchOne(ctx, "ch-b")
	require.NoError(t, err)
	assert.Equal(t, []byte("msg-b"), resultB)
}

func TestPublishAndFetchMany_LargeMessages(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "large-channel"

	// Create a 1MB message
	largeMsg := make([]byte, 1024*1024)
	for i := range largeMsg {
		largeMsg[i] = byte(i % 256)
	}

	err := adapter.Publish(ctx, channel, largeMsg)
	require.NoError(t, err)

	result, err := adapter.FetchOne(ctx, channel)
	require.NoError(t, err)
	assert.Equal(t, largeMsg, result)
}

func TestFetchOne_ConsumesMessage(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "consume-channel"

	err := adapter.Publish(ctx, channel, []byte("once"))
	require.NoError(t, err)

	// First fetch returns the message
	result, err := adapter.FetchOne(ctx, channel)
	require.NoError(t, err)
	assert.Equal(t, []byte("once"), result)

	// Second fetch should return nil (consumed)
	result, err = adapter.FetchOne(ctx, channel)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestFetchMany_ConsumesMessages(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "consume-many"

	for i := 0; i < 3; i++ {
		err := adapter.Publish(ctx, channel, []byte(fmt.Sprintf("m-%d", i)))
		require.NoError(t, err)
	}

	// Fetch all 3
	messages, err := adapter.FetchMany(ctx, channel, 3)
	require.NoError(t, err)
	require.Len(t, messages, 3)

	// Queue should be empty now
	messages, err = adapter.FetchMany(ctx, channel, 10)
	assert.NoError(t, err)
	assert.Nil(t, messages)
}

func TestSetChannelTTL(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "ttl-channel"

	// Publish messages so the key exists
	for i := 0; i < 3; i++ {
		err := adapter.Publish(ctx, channel, []byte(fmt.Sprintf("ttl-%d", i)))
		require.NoError(t, err)
	}

	// Set a short TTL (2 seconds)
	err := adapter.SetChannelTTL(ctx, channel, 2)
	require.NoError(t, err)

	// Verify the TTL is set using the raw Valkey client
	key := adapter.cfg.Prefix + channel
	ttlCmd := adapter.coreValkey.B().Ttl().Key(key).Build()
	ttl, err := adapter.coreValkey.Do(ctx, ttlCmd).AsInt64()
	require.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= 2, "TTL should be between 1 and 2, got %d", ttl)

	// Messages should still be readable before expiry
	length, err := adapter.Len(ctx, channel)
	require.NoError(t, err)
	assert.Equal(t, int64(3), length)

	// Wait for TTL to expire
	time.Sleep(3 * time.Second)

	// Channel should be gone
	length, err = adapter.Len(ctx, channel)
	require.NoError(t, err)
	assert.Equal(t, int64(0), length)
}

func TestSetChannelTTL_NonExistentChannel(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()

	// SetChannelTTL on a non-existent key should not error (EXPIRE returns 0 but no error)
	err := adapter.SetChannelTTL(ctx, "no-such-channel", 10)
	assert.NoError(t, err)
}

func TestSetChannelTTL_RefreshExtendsTTL(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "refresh-ttl"

	err := adapter.Publish(ctx, channel, []byte("data"))
	require.NoError(t, err)

	// Set short TTL
	err = adapter.SetChannelTTL(ctx, channel, 2)
	require.NoError(t, err)

	// Wait 1 second then refresh with longer TTL
	time.Sleep(1 * time.Second)
	err = adapter.SetChannelTTL(ctx, channel, 10)
	require.NoError(t, err)

	// Verify TTL was extended
	key := adapter.cfg.Prefix + channel
	ttlCmd := adapter.coreValkey.B().Ttl().Key(key).Build()
	ttl, err := adapter.coreValkey.Do(ctx, ttlCmd).AsInt64()
	require.NoError(t, err)
	assert.True(t, ttl > 2, "TTL should have been refreshed to >2, got %d", ttl)
}

func TestLen(t *testing.T) {
	adapter, cleanup := setupValkeyContainer(t)
	defer cleanup()

	ctx := context.Background()
	channel := "len-channel"

	// Empty queue should have length 0
	length, err := adapter.Len(ctx, channel)
	require.NoError(t, err)
	assert.Equal(t, int64(0), length)

	// Publish 3 messages
	for i := 0; i < 3; i++ {
		err := adapter.Publish(ctx, channel, []byte(fmt.Sprintf("len-%d", i)))
		require.NoError(t, err)
	}

	// Length should be 3
	length, err = adapter.Len(ctx, channel)
	require.NoError(t, err)
	assert.Equal(t, int64(3), length)

	// Consume one, length should be 2
	_, err = adapter.FetchOne(ctx, channel)
	require.NoError(t, err)

	length, err = adapter.Len(ctx, channel)
	require.NoError(t, err)
	assert.Equal(t, int64(2), length)
}
