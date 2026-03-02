package valkey

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/queue"
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

func New(cfg *Config) queue.Adapter {
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

func (va *valkeyAdapter) Publish(ctx context.Context, channel string, message []byte) error {
	key := va.cfg.Prefix + channel
	cmd := va.coreValkey.B().Rpush().Key(key).Element(valkey.BinaryString(message)).Build()
	return va.coreValkey.Do(ctx, cmd).Error()
}

func (va *valkeyAdapter) SetChannelTTL(ctx context.Context, channel string, ttlInSec int64) error {
	key := va.cfg.Prefix + channel
	cmd := va.coreValkey.B().Expire().Key(key).Seconds(ttlInSec).Build()
	return va.coreValkey.Do(ctx, cmd).Error()
}

func (va *valkeyAdapter) FetchOne(ctx context.Context, channel string) (message []byte, err error) {
	key := va.cfg.Prefix + channel

	cmd := va.coreValkey.B().Lpop().Key(key).Build()
	result, err := va.coreValkey.Do(ctx, cmd).ToString()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}

	return []byte(result), nil
}

func (va *valkeyAdapter) FetchMany(ctx context.Context, channel string, maxItem int) (messages [][]byte, err error) {
	key := va.cfg.Prefix + channel

	cmd := va.coreValkey.B().Lpop().Key(key).Count(int64(maxItem)).Build()
	result, err := va.coreValkey.Do(ctx, cmd).AsStrSlice()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}

	messages = make([][]byte, 0, len(result))
	for _, item := range result {
		messages = append(messages, []byte(item))
	}

	return messages, nil
}

func (va *valkeyAdapter) BlockFetchOne(ctx context.Context, channel string, timeout time.Duration) ([]byte, error) {
	key := va.cfg.Prefix + channel
	seconds := timeout.Seconds()

	cmd := va.coreValkey.B().Blpop().Key(key).Timeout(seconds).Build()
	result, err := va.coreValkey.Do(ctx, cmd).AsStrSlice()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}

	if len(result) < 2 {
		return nil, nil
	}
	return []byte(result[1]), nil
}

func (va *valkeyAdapter) Len(ctx context.Context, channel string) (int64, error) {
	key := va.cfg.Prefix + channel
	cmd := va.coreValkey.B().Llen().Key(key).Build()
	return va.coreValkey.Do(ctx, cmd).AsInt64()
}

func (va *valkeyAdapter) Healthcheck(ctx context.Context) error {
	cmd := va.coreValkey.B().Ping().Build()
	err := va.coreValkey.Do(ctx, cmd).Error()
	if err != nil {
		return fmt.Errorf("Valkey is not connected: %w", err)
	}
	return nil
}

func (va *valkeyAdapter) Close() {
	va.coreValkey.Close()
}
