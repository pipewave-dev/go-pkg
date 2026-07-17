## Run as a container

`pipewave-server` (`cmd/pipewave-server`) packages the pipewave realtime core
as a standalone container: a REST admin API replaces direct Go method calls,
and signed HTTP webhooks replace the injected `types.Fns` hooks, so
non-Go backends (Node, Python, PHP, Java, ...) can integrate without writing a
Go host. See the design doc for the full rationale and REST mapping table:
[`docs/feats/2026-07-17-rest-api-container-design.md`](docs/feats/2026-07-17-rest-api-container-design.md).

### Config

Copy [`server-config.example.yaml`](server-config.example.yaml) and point
`SERVER.CALLBACKS.BASE_URL` at your application backend's callback endpoint.
Scalar fields can also be overridden via `APP_`-prefixed env vars (e.g.
`APP_SERVER__ADMIN_ADDR`, `APP_SERVER__CALLBACKS__BASE_URL`).

### Callback contract

Every webhook is an HTTP POST to `SERVER.CALLBACKS.BASE_URL` with a signed
JSON envelope:

```json
{"data": {...}, "meta": {"sent_at": 1737100000000, "id": "cb_...", "event_type": "..."}}
```

The `X-Pipewave-Signature` header carries the base64 Ed25519 signature over
the raw request body; verify it against the container's public key
(`GET /api/v1/webhook/public-key`). Reject stale deliveries (`meta.sent_at`
too far in the past) to guard against replay.

Event types:

| `event_type`        | Class            | Expected response                                                          |
| -------------------- | ---------------- | --------------------------------------------------------------------------- |
| `inspect_token`       | sync, fail-closed | `{"user_id","is_anonymous","metadata"}`                                     |
| `handle_message`      | sync (or async in `forward`/`disabled` mode) | `{"output_type","data"}` (`data` is base64 in JSON)     |
| `on_new_connection`   | sync, fail-closed | any 2xx admits the connection; non-2xx rejects it                          |
| `on_new_connection_established`, `on_close_connection`, `on_read_error`, `on_write_error`, `message_received` (forward mode) | async, retried | any 2xx acknowledges receipt |

A minimal reference receiver implementing this contract, in Go, using only
the standard library HTTP/crypto APIs (portable to any language), lives at
[`examples/rest-backend/main.go`](examples/rest-backend/main.go).

### Build and run

```bash
docker build -t pipewave-server:dev .
```

A `pipewave-server` service is included in [`docker-compose.yml`](docker-compose.yml),
wired to the same `valkey`/`postgres` services used for local development.

### Smoke test

```bash
# terminal 1: infra
docker compose up -d valkey postgres

# terminal 2: stub backend
go run ./examples/rest-backend -addr :9000

# terminal 3: the server (config points BASE_URL at localhost:9000)
go run ./cmd/pipewave-server -config server-config.example.yaml

# terminal 4: drive it
# 4a. health
curl -s localhost:8081/healthz                                # {"healthy":true}
# 4b. admin auth
curl -s localhost:8081/api/v1/webhook/public-key              # 401
curl -s -H "Authorization: Bearer change-me" localhost:8081/api/v1/webhook/public-key
# 4c. websocket roundtrip (token "alice" -> inspect_token callback -> user alice)
#     issue-tmp-token then connect; easiest with the playground web client or websocat.
#     Non-anonymous callers must also pass X-Pipewave-ID (client-chosen instance/session id).
curl -s -X POST -H "Authorization: Bearer alice" -H "X-Pipewave-ID: alice-device-1" \
  localhost:8080/issue-tmp-token                               # -> {"connToken":"..."}
websocat "ws://localhost:8080/gw?tk=<connToken>"
#     send a frame; expect the echo response produced by the stub backend,
#     and handle_message / on_new_connection lines in terminal 2's log.
# 4d. REST send while the socket is open
curl -s -X POST -H "Authorization: Bearer change-me" -H "Content-Type: application/json" \
  -d '{"user_id":"Bearer alice","msg_type":"NEWS","payload":"aGVsbG8="}' \
  localhost:8081/api/v1/messages/user                          # {"sent":true}
#     the frame appears on the websocket.
# 4e. presence
curl -s -H "Authorization: Bearer change-me" "localhost:8081/api/v1/presence/Bearer%20alice"  # {"online":true}
# 4f. close the socket -> on_close_connection appears in terminal 2's log.
```

Note: the WebSocket wire frame on `/gw` is msgpack, not JSON — see
`core/service/websocket/0.message_type.go` (`WebsocketResquest`/`WebsocketResponse`,
fields `i`/`t`/`b`/`r`/`e`/`a`); `t` (`MsgType`) is the same string as
`handle_message`'s `input_type`/`output_type` in the callback contract above.
