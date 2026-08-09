// Package natsjs cung cấp một pubsub publisher dựa trên NATS JetStream.
//
// Khác với adapter Valkey (at-most-once, không ack), JetStream ack sau khi
// message đã persist — đủ điều kiện làm callback transport.
package natsjs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// connectTimeout giới hạn thời gian dial ban đầu tới NATS. Mặc định của
// nats.go là 2s và không retry vô hạn lúc connect lần đầu
// (RetryOnFailedConnect mặc định false), nhưng ta rút ngắn timeout để New
// fail nhanh hơn khi broker không reachable.
const connectTimeout = 2 * time.Second

type Config struct {
	URL           string
	Stream        string
	SubjectPrefix string
}

type Adapter struct {
	conn *nats.Conn
	js   jetstream.JetStream
}

// New kết nối tới NATS, bật JetStream và đảm bảo stream tồn tại
// (idempotent: tạo nếu chưa có, cập nhật subject nếu đã có).
func New(cfg *Config) (*Adapter, error) {
	conn, err := nats.Connect(cfg.URL, nats.Timeout(connectTimeout))
	if err != nil {
		return nil, fmt.Errorf("natsjs: connect %s: %w", cfg.URL, err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("natsjs: jetstream: %w", err)
	}

	if cfg.Stream != "" {
		if cfg.SubjectPrefix == "" {
			conn.Close()
			return nil, fmt.Errorf("natsjs: Stream %q set without SubjectPrefix", cfg.Stream)
		}
		subjects := []string{cfg.SubjectPrefix + ".>"}
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:     cfg.Stream,
			Subjects: subjects,
			Storage:  jetstream.FileStorage,
		})
		cancel()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("natsjs: ensure stream %s: %w", cfg.Stream, err)
		}
	}

	return &Adapter{conn: conn, js: js}, nil
}

// Publish gửi message và CHỜ JetStream ack — trả nil nghĩa là đã persist.
// Msg-Id được set từ callback ID trong envelope để JetStream dedupe khi
// có retry.
func (a *Adapter) Publish(ctx context.Context, subject string, payload []byte) error {
	msg := &nats.Msg{Subject: subject, Data: payload}
	if id := callbackIDFrom(payload); id != "" {
		msg.Header = nats.Header{jetstream.MsgIDHeader: []string{id}}
	}
	if _, err := a.js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("natsjs: publish %s: %w", subject, err)
	}
	return nil
}

func (a *Adapter) Healthcheck() error {
	if !a.conn.IsConnected() {
		return fmt.Errorf("natsjs: not connected")
	}
	return nil
}

func (a *Adapter) Close() {
	if a.conn != nil {
		a.conn.Close()
	}
}

// callbackIDFrom rút meta.id khỏi envelope để dùng làm Nats-Msg-Id.
// Lỗi parse trả "" — publish vẫn tiếp tục, chỉ mất khả năng dedupe.
func callbackIDFrom(payload []byte) string {
	var env struct {
		Meta struct {
			ID string `json:"id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return ""
	}
	return env.Meta.ID
}
