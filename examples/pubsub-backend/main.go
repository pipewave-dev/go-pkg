// Command pubsub-backend là backend mẫu cho chế độ hybrid:
//   - Class-2 (async) nhận qua NATS JetStream.
//   - Class-1 (sync) vẫn nhận qua HTTP webhook, vì chúng cần response.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type meta struct {
	SentAt    int64  `json:"sent_at"`
	ID        string `json:"id"`
	EventType string `json:"event_type"`
}

type envelope struct {
	Data json.RawMessage `json:"data"`
	Meta meta            `json:"meta"`
}

func main() {
	natsURL := envOr("NATS_URL", "nats://localhost:29103")
	addr := envOr("ADDR", ":9000")

	go serveClass1(addr)

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("connect nats: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	ctx := context.Background()
	cons, err := js.CreateOrUpdateConsumer(ctx, "PIPEWAVE_EVENTS", jetstream.ConsumerConfig{
		Durable:       "pubsub-backend",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: "pipewave.events.>",
	})
	if err != nil {
		log.Fatalf("create consumer: %v", err)
	}

	log.Printf("subscribed to pipewave.events.> on %s", natsURL)
	_, err = cons.Consume(func(msg jetstream.Msg) {
		var env envelope
		if err := json.Unmarshal(msg.Data(), &env); err != nil {
			log.Printf("bad envelope on %s: %v", msg.Subject(), err)
			_ = msg.Ack() // không nak: message hỏng, retry cũng vô ích
			return
		}
		switch env.Meta.EventType {
		case "on_new_connection_established", "on_close_connection",
			"on_read_error", "on_write_error", "message_received":
			log.Printf("event=%s id=%s data=%s", env.Meta.EventType, env.Meta.ID, env.Data)
		default:
			log.Printf("unhandled event=%s id=%s", env.Meta.EventType, env.Meta.ID)
		}
		_ = msg.Ack()
	})
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	select {}
}

// serveClass1 phục vụ các callback sync (inspect_token, handle_message,
// on_new_connection) — pubsub KHÔNG thay thế được vì chúng cần response.
func serveClass1(addr string) {
	http.HandleFunc("/pipewave/callback", func(w http.ResponseWriter, r *http.Request) {
		var env envelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch env.Meta.EventType {
		case "inspect_token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user_id": "demo-user", "is_anonymous": false,
				"metadata": map[string]string{},
			})
		case "handle_message":
			// echo lại nguyên payload
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output_type": "text", "data": []byte("pong"),
			})
		default: // on_new_connection: 2xx là chấp nhận kết nối
			w.WriteHeader(http.StatusOK)
		}
	})
	log.Printf("class-1 webhook listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("http: %v", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
