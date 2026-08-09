package natsjs_test

import (
	"context"
	"testing"

	"github.com/pipewave-dev/go-pkg/pkg/pubsub/adapters/natsjs"
	"github.com/stretchr/testify/require"
)

// Adapter phải thoả bề mặt Publisher mà server/callback yêu cầu.
//
// CỐ Ý khai báo lại interface tại đây thay vì import server/callback:
// pkg/ không được phụ thuộc server/ (hướng phụ thuộc đi một chiều
// server/ -> pkg/). Structural typing của Go vẫn cho ta kiểm tra
// contract này lúc compile.
type publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
	Healthcheck() error
}

func TestAdapterSatisfiesPublisher(t *testing.T) {
	var _ publisher = (*natsjs.Adapter)(nil)
}

// Không kết nối được thì phải lỗi ngay lúc New, không phải im lặng.
func TestNewFailsOnUnreachableBroker(t *testing.T) {
	_, err := natsjs.New(&natsjs.Config{
		URL:    "nats://127.0.0.1:1",
		Stream: "TEST_STREAM",
	})
	require.Error(t, err)
}
