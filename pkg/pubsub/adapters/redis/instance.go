package redis

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	"github.com/valkey-io/valkey-go"
)

type redisAdapter struct {
	cfg *Config

	coreRedis valkey.Client
}

type Config struct {
	RedisEndpoint string
	Password      string
	DB            int
	Prefix        string

	// MaxSubscribeRetries is how many consecutive times Subscribe will retry a dropped
	// subscription (with backoff) before giving up and invoking CallbackFn. Defaults to
	// defaultMaxSubscribeRetries if <= 0.
	MaxSubscribeRetries int
	// CallbackFn is invoked once a subscription could not be re-established after
	// MaxSubscribeRetries attempts, so the caller can react (e.g. restart the process).
	CallbackFn func()
}

const defaultMaxSubscribeRetries = 3

// subscribeRetryBackoff returns an exponential backoff (capped at 5s) with jitter for the
// given retry attempt (1-indexed).
func subscribeRetryBackoff(attempt int) time.Duration {
	const (
		base       = 500 * time.Millisecond
		maxBackoff = 5 * time.Second
	)

	d := min(base*time.Duration(1<<uint(attempt-1)), maxBackoff)

	return d/2 + time.Duration(rand.Int63n(int64(d/2)+1))
}

func New(cfg *Config) pubsub.Adapter {
	ins := &redisAdapter{
		cfg: cfg,
	}
	ins.connect()
	return ins
}

func (ra *redisAdapter) connect() {
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{ra.cfg.RedisEndpoint},
		Password:    ra.cfg.Password,
		SelectDB:    ra.cfg.DB,
	})
	if err != nil {
		slog.Error("Failed to connect to Redis", slog.Any("err", err))
		panic("Failed to connect to Redis")
	}
	ra.coreRedis = client
}

func (ra *redisAdapter) Flush() {
	ra.coreRedis.Close()
}

func (ra *redisAdapter) Subscribe(channel string, handler func(message []byte)) (unsubscribe func(), err error) {
	fullChannel := ra.cfg.Prefix + channel

	// Create a new context for the subscription goroutine
	subCtx, cancel := context.WithCancel(context.Background())

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Redis subscription panic", slog.Any("panic", r))
			}
		}()

		maxRetries := ra.cfg.MaxSubscribeRetries
		if maxRetries <= 0 {
			maxRetries = defaultMaxSubscribeRetries
		}

		attempt := 0
		for subCtx.Err() == nil {
			receiveErr := ra.coreRedis.Receive(subCtx, ra.coreRedis.B().Subscribe().Channel(fullChannel).Build(), func(msg valkey.PubSubMessage) {
				if msg.Message != "" {
					handler([]byte(msg.Message))
				}
			})
			if subCtx.Err() != nil {
				return
			}

			attempt++
			slog.Error("Redis subscription dropped, retrying",
				slog.String("channel", fullChannel),
				slog.Int("attempt", attempt),
				slog.Any("err", receiveErr))

			if attempt >= maxRetries {
				slog.Error("Redis subscription permanently dead after retries",
					slog.String("channel", fullChannel),
					slog.Int("attempts", attempt))
				if ra.cfg.CallbackFn != nil {
					ra.cfg.CallbackFn()
				}
				return
			}

			select {
			case <-time.After(subscribeRetryBackoff(attempt)):
			case <-subCtx.Done():
				return
			}
		}
	}()

	unsubscribe = func() {
		cancel()
	}

	return unsubscribe, nil
}

func (ra *redisAdapter) Publish(ctx context.Context, channel string, message []byte) error {
	fullChannel := ra.cfg.Prefix + channel

	cmd := ra.coreRedis.B().Publish().Channel(fullChannel).Message(string(message)).Build()
	return ra.coreRedis.Do(ctx, cmd).Error()
}

func (ra *redisAdapter) Healthcheck() error {
	ctx := context.Background()
	cmd := ra.coreRedis.B().Ping().Build()
	err := ra.coreRedis.Do(ctx, cmd).Error()
	if err != nil {
		return fmt.Errorf("Redis is not connected: %w", err)
	}
	return nil
}
