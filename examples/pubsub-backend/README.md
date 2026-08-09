# pubsub-backend

Backend mẫu cho chế độ **hybrid**:

- **Class-2** (`on_close_connection`, `on_read_error`, `on_write_error`,
  `message_received`, `on_new_connection_established`) — nhận qua **NATS JetStream**,
  subject `pipewave.events.>`.
- **Class-1** (`inspect_token`, `handle_message`, `on_new_connection`) — vẫn nhận
  qua **HTTP webhook**, vì chúng cần giá trị trả về trên hot path.

## Chạy

```bash
docker compose up -d nats postgres valkey
go run ./examples/pubsub-backend      # lắng nghe :9000 + subscribe NATS
docker compose up pipewave-server
```

Env: `NATS_URL` (mặc định `nats://localhost:29103`), `ADDR` (mặc định `:9000`).
