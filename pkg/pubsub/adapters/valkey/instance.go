package valkey

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/pubsub"
	"github.com/valkey-io/valkey-go"
)

type valkeyAdapter struct {
	cfg *Config

	coreValkey valkey.Client
}

type Config struct {
	ValkeyEndpoint string
	Password       string
	DB             int
	Prefix         string

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
	ins := &valkeyAdapter{
		cfg: cfg,
	}
	ins.connect()
	return ins
}

func (va *valkeyAdapter) connect() {
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{va.cfg.ValkeyEndpoint},
		Password:    va.cfg.Password,
		SelectDB:    va.cfg.DB,
	})
	if err != nil {
		slog.Error("Failed to connect to Valkey", slog.Any("err", err))
		panic("Failed to connect to Valkey")
	}
	va.coreValkey = client
}

func (va *valkeyAdapter) Flush() {
	va.coreValkey.Close()
}

func (va *valkeyAdapter) Subscribe(channel string, handler func(message []byte)) (unsubscribe func(), err error) {
	fullChannel := va.cfg.Prefix + channel

	// Create a new context for the subscription goroutine
	subCtx, cancel := context.WithCancel(context.Background())

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Valkey subscription panic", slog.Any("panic", r))
			}
		}()

		maxRetries := va.cfg.MaxSubscribeRetries
		if maxRetries <= 0 {
			maxRetries = defaultMaxSubscribeRetries
		}

		attempt := 0
		for subCtx.Err() == nil {
			receiveErr := va.coreValkey.Receive(subCtx, va.coreValkey.B().Subscribe().Channel(fullChannel).Build(), func(msg valkey.PubSubMessage) {
				if msg.Message != "" {
					handler([]byte(msg.Message))
				}
			})
			if subCtx.Err() != nil {
				return
			}

			attempt++
			slog.Error("Valkey subscription dropped, retrying",
				slog.String("channel", fullChannel),
				slog.Int("attempt", attempt),
				slog.Any("err", receiveErr))

			if attempt >= maxRetries {
				slog.Error("Valkey subscription permanently dead after retries",
					slog.String("channel", fullChannel),
					slog.Int("attempts", attempt))
				if va.cfg.CallbackFn != nil {
					va.cfg.CallbackFn()
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

func (va *valkeyAdapter) Publish(ctx context.Context, channel string, message []byte) error {
	fullChannel := va.cfg.Prefix + channel

	cmd := va.coreValkey.B().Publish().Channel(fullChannel).Message(valkey.BinaryString(message)).Build()
	return va.coreValkey.Do(ctx, cmd).Error()
}

func (va *valkeyAdapter) Healthcheck() error {
	ctx := context.Background()
	cmd := va.coreValkey.B().Ping().Build()
	err := va.coreValkey.Do(ctx, cmd).Error()
	if err != nil {
		return fmt.Errorf("Valkey is not connected: %w", err)
	}
	return nil
}
