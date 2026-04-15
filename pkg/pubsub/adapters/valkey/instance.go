package valkey

import (
	"context"
	"fmt"
	"log/slog"

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

		err = va.coreValkey.Receive(subCtx, va.coreValkey.B().Subscribe().Channel(fullChannel).Build(), func(msg valkey.PubSubMessage) {
			if msg.Message != "" {
				handler([]byte(msg.Message))
			}
		})
		if err != nil && subCtx.Err() == nil {
			slog.Error("Valkey subscription error", slog.Any("err", err))
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
