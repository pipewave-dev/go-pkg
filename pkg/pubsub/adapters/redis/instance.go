package redis

import (
	"context"
	"fmt"
	"log/slog"

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

		err := ra.coreRedis.Receive(subCtx, ra.coreRedis.B().Subscribe().Channel(fullChannel).Build(), func(msg valkey.PubSubMessage) {
			if msg.Message != "" {
				handler([]byte(msg.Message))
			}
		})
		if err != nil && subCtx.Err() == nil {
			slog.Error("Redis subscription error", slog.Any("err", err))
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
