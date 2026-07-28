# pipewave-server

`pipewave-server` packages the pipewave realtime core as a **standalone
container**. Instead of importing this repo as a Go library, your application
backend talks to pipewave over HTTP in both directions:

- **You → pipewave:** a REST admin API (`/api/v1/...`) replaces direct calls
  to the `ExportedServices` Go interface (send message, disconnect, presence,
  monitoring, ...).
- **Pipewave → you:** signed HTTP callbacks (webhooks) replace the injected
  `types.Fns` Go hooks (token inspection, message handling, connection
  lifecycle events).

This makes pipewave usable from any language (Node, Python, PHP, Java, ...).
The end-client WebSocket/long-polling surface is unchanged — browsers and
mobile apps connect to the same `/gw`, `/lp`, `/issue-tmp-token` endpoints as
in embedded mode.

Design doc: [`docs/feats/2026-07-17-rest-api-container-design.md`](../docs/feats/2026-07-17-rest-api-container-design.md)
· Smoke-test walkthrough: [root `README.md`](../README.md)
· Reference callback receiver: [`examples/rest-backend/main.go`](../examples/rest-backend/main.go)

---

## 1. Embedded mode vs container mode

|                    | Embedded (Go package)                                              | Container (`pipewave-server`)                           |
| ------------------ | ------------------------------------------------------------------ | ------------------------------------------------------- |
| Integration        | `pipewave.New(...)`, mount `pw.Mux()`, implement `types.Fns` in Go | run the container, implement one HTTP callback endpoint |
| Server → app calls | Go interfaces (`Fns.InspectToken`, `Fns.HandleMessage`, ...)       | signed webhook POSTs to `SERVER.CALLBACKS.BASE_URL`     |
| App → server calls | `pw.Services().SendToUser(...)` etc.                               | `POST /api/v1/messages/user` etc. (Bearer API key)      |
| Language           | Go only                                                            | any                                                     |
| Example            | [`playground/`](../playground)                                     | [`examples/rest-backend/`](../examples/rest-backend)    |

Both modes run the **same core**. The container is strictly a wrapper: nothing
under `core/`, `provider/`, `export/`, or `shared/` was modified to build it.

---

## 2. Architecture

```
┌─────────────────────┐              ┌────────────────────────────────────┐
│ WS client (browser) │   yourApi    │ App Backend (any language)         │
│  App backend API────┼─────────────►│                                    │
│                     │              │                   callback endpoint│◄─────────────────┐
│  /gw /lp            │  pipewave    │                                    │pipewave-admin    │
│  /issue-tmp-token ──┼──────────┐   │Logic app call "Pipewave server API"├─────────────┐    │
└─────────────────────┘          │   └────────────────────────────────────┘             │    │
                                 │                                                      │    │
                                 │   ┌─────────────────────────────────────┐            │    │
                                 │   │ Pipewave server                     │            │    │
                                 │   │                                     │            │    │
                                 └──►│:8080                           :8081│◄───────────┘    │
                                     │(client listener)    (admin listener)│                 │
                                     │                                     │                 │
                                     ├─────────────────────────────────────┤                 │
                                     │(pipewave core - golang)             │                 │
                                     │                                     │                 │
                                     │ delivery.ModuleDelivery             │                 │
                                     │ (core unchanged) ◄── SetFns(hooks)  │                 │
                                     │                               │     │                 │
                                     │                     (hooks) ◄─┘     │                 │
                                     │                       (sync caller) ┼─────────────────┘
                                     │                                     │
                                     └────┬──────────────────┬─────────────┘
                                          │                  │
                                     ┌────▼───┐     ┌────────▼──── ─────┐
                                     │ Valkey │     │ Postgres/DynamoDB │
                                     └────────┘     └───────────────────┘
```

Two listeners so operators can network-isolate them:

- **Client port** (`SERVER.CLIENT_ADDR`, default `:8080`) — public. Mounts the
  core's `pw.Mux()` unchanged: `POST /issue-tmp-token`, `/gw` (WebSocket),
  `POST /lp`, `POST /lp-send`.
- **Admin port** (`SERVER.ADMIN_ADDR`, default `:8081`) — private. The REST
  admin API (API-key authenticated) plus unauthenticated `GET /healthz`.

---

## 3. Package structure

```
server/
├── config/     SERVER: section of the YAML config — loading, defaults, validation
├── webhook/    everything about outbound callbacks:
│   ├── envelope.go   envelope types (Body/Meta), event-type constants, callback IDs
│   ├── signer.go     Ed25519 signer — key load/generate/persist, Sign/Verify/PublicKey
│   ├── sender.go     one signed HTTP POST (no retry) — shared by sync + async paths
│   ├── sync.go       SyncCaller (request/response callbacks) + CircuitBreaker
│   └── async.go      AsyncDispatcher (fire-and-forget events, backoff retry queue)
├── authn/      JWT inspect-token mode (JWKS URL or static PEM) — optional
├── fns/        the bridge: builds a *types.Fns whose hooks call the webhook layer
└── restapi/    admin REST API: mux + handlers wrapping ExportedServices/Monitoring

cmd/pipewave-server/   the binary: config → core → wiring → two listeners
examples/rest-backend/ minimal callback receiver (stdlib only, porting reference)
```

Responsibilities, bottom-up:

| Package               | Exports (what other layers use)                                                                         | Depends on                                                                             |
| --------------------- | ------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| `server/config`       | `Load(yamlFiles) (*ServerConfigT, error)`, mode/repository constants                                    | `pkg/koanf`                                                                            |
| `server/webhook`      | `Signer`, `Sender`, `SyncCaller`+`CircuitBreaker`, `AsyncDispatcher`, envelope types, event constants   | — (stdlib + nanoid)                                                                    |
| `server/authn`        | `NewJWTInspector(ctx, JWTConfig)` → `InspectToken(...)` matching the `types.Fns.InspectToken` signature | `golang-jwt/jwt/v5`, `keyfunc/v3`                                                      |
| `server/fns`          | `New(syncCaller, async, Config) *types.Fns`                                                             | `server/webhook`, `server/config`, `export/types`                                      |
| `server/restapi`      | `NewAdminMux(pw delivery.ModuleDelivery, MuxConfig) *http.ServeMux`                                     | `core/delivery` (interfaces only), `shared/aerror`, `server/webhook` (public-key type) |
| `cmd/pipewave-server` | the binary                                                                                              | all of the above + `export/adapters/*`                                                 |

---

## 4. How the server uses the core

The wrapper consumes **only the public surface** a normal Go embedder would
use — the same contract as `playground/`:

1. **Construction** (`cmd/pipewave-server/main.go`): `pipewave.New(PipewaveConfig{...})`
   with the standard adapters — repository chosen by `SERVER.REPOSITORY`
   (`export/adapters/repo/postgresql` or `.../dynamodb`), queue and pubsub
   always Valkey (`export/adapters/queue/valkey`, `.../pubsub/valkey`). Core
   config (VALKEY, POSTGRES, WORKER_POOL, ...) is read from the same YAML
   files via `pipewave.ConfigFromYAML`.
2. **Hooks** — the one _inversion_ point: `pw.SetFns(serverfns.New(...))`
   installs a `*types.Fns` whose implementations POST to your backend instead
   of running Go code (section 6).
3. **Methods** — `restapi.NewAdminMux(pw, ...)` wraps `pw.Services()`
   (`delivery.ExportedServices`), `pw.Monitoring()`, and `pw.IsHealthy()` as
   REST endpoints (section 7).
4. **Lifecycle** — `pw.RunMigration()` before listening; on SIGINT/SIGTERM:
   HTTP servers shut down, then `pw.Shutdown()` (core drain), then the async
   webhook queue flushes best-effort (30s overall deadline).

Because only `delivery.ModuleDelivery` is consumed, core changes that keep
that interface stable cannot break the server.

**Go embedders can reuse the admin API too:** `restapi.NewAdminMux(pw, cfg)`
is an ordinary `*http.ServeMux` built from the public surface — mount it into
your own server if you want the HTTP admin surface in embedded mode.

---

## 5. Configuration (`SERVER:` section)

Lives in the same YAML file(s) as the core config; pass files with
`-config a.yaml,b.yaml` (later files override earlier ones — this is the
supported override mechanism; `APP_*` env-var overrides are currently
non-functional due to a known `pkg/koanf` bug). Full example:
[`server-config.example.yaml`](../server-config.example.yaml).

| Key                                                  | Default               | Meaning                                                                  |
| ---------------------------------------------------- | --------------------- | ------------------------------------------------------------------------ |
| `CLIENT_ADDR`                                        | `:8080`               | public client listener                                                   |
| `ADMIN_ADDR`                                         | `:8081`               | private admin listener                                                   |
| `API_KEYS`                                           | — (required)          | static admin API keys; several allowed for rotation                      |
| `REPOSITORY`                                         | `postgres`            | `postgres` \| `dynamodb`                                                 |
| `AUTH.MODE`                                          | — (required)          | `webhook` \| `jwt` (section 6.1)                                         |
| `AUTH.JWT.JWKS_URL` / `AUTH.JWT.PUBLIC_KEY_PEM_FILE` | —                     | key source for `jwt` mode (exactly one required)                         |
| `AUTH.JWT.USER_ID_CLAIM`                             | `sub`                 | claim mapped to `user_id`                                                |
| `AUTH.JWT.METADATA_CLAIMS`                           | `[]`                  | string claims copied into connection metadata                            |
| `CALLBACKS.BASE_URL`                                 | — (required)          | single URL that receives **all** callbacks                               |
| `CALLBACKS.SIGNING_KEY_FILE`                         | `webhook_ed25519.key` | Ed25519 seed (base64); generated with 0600 perms on first boot if absent |
| `CALLBACKS.HANDLE_MESSAGE.MODE`                      | `sync`                | `sync` \| `forward` \| `disabled` (section 6.2)                          |
| `CALLBACKS.HANDLE_MESSAGE.TIMEOUT`                   | `5s`                  | sync handle_message deadline                                             |
| `CALLBACKS.SYNC_TIMEOUT`                             | `3s`                  | inspect_token / on_new_connection deadline                               |
| `CALLBACKS.ASYNC_RETRY_MAX`                          | `6`                   | delivery attempts per async event                                        |
| `CALLBACKS.SYNC_RETRY.MAX`                           | `1`                   | sync callback attempts (`inspect_token`/`on_new_connection`/sync `handle_message`); `1` = no retry, since a client is waiting |
| `CALLBACKS.SYNC_RETRY.BACKOFF`                       | `100ms`               | delay between sync retry attempts (only used when `MAX > 1`)             |
| `CALLBACKS.BREAKER.THRESHOLD`                        | `5`                   | consecutive transport/5xx failures before the circuit opens              |
| `CALLBACKS.BREAKER.COOLDOWN`                         | `10s`                 | how long the breaker stays open before allowing a probe request          |
| `CALLBACKS.ASYNC_BACKOFF`                            | `[1s,5s,30s,2m,10m]`  | per-attempt backoff for async events (last value repeats); empty = use this default |
| `CALLBACKS.PING.ENABLED`                             | `false`               | enable periodic + boot-time backend health pings                        |
| `CALLBACKS.PING.PATH`                                | `/pipewave/ping`      | path pipewave appends to `BASE_URL`'s host for ping requests (ping only) |
| `CALLBACKS.PING.INTERVAL`                            | `30s`                 | runtime ping period (ping only)                                          |
| `CALLBACKS.PING.TIMEOUT`                             | `3s`                  | per-ping request deadline (ping only)                                    |
| `CALLBACKS.PING.FAIL_THRESHOLD`                      | `3`                   | consecutive ping failures before the backend is marked unhealthy (ping only) |
| `CALLBACKS.UNHEALTHY_ACTION`                         | `log-only`            | `shutdown` \| `log-only` — what to do when the backend is marked unhealthy |
| `CALLBACKS.BREAKER_OPEN_SHUTDOWN`                    | `0s` (disabled)       | if `> 0`, the breaker staying continuously open this long also marks the backend unhealthy |

---

## 6. Callbacks (pipewave → your backend)

Everything POSTs to `CALLBACKS.BASE_URL` with one envelope; switch on
`meta.event_type`:

```json
{
  "data": { ...event-specific... },
  "meta": { "sent_at": 1737100000000, "id": "cb_...", "event_type": "..." }
}
```

- `X-Pipewave-Signature` header = base64 **Ed25519 signature over the raw
  request body**. Fetch the public key once from
  `GET /api/v1/webhook/public-key` → `{"alg":"Ed25519","public_key_in_base64":"..."}`.
  Verify the signature, and reject envelopes whose `meta.sent_at` (unix ms) is
  too old (recommend 5 min) to block replays.
- `meta.id` is the idempotency key — **retries of an async event reuse the
  same id**, so dedupe on it.

### 6.1 Class 1 — synchronous (pipewave blocks on your HTTP response)

| `event_type`                 | `data`                                                                                                                    | expected 2xx response body                                              |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| `inspect_token`              | `{"token","headers"}` — `token` is the **raw** `Authorization` header value, `Bearer ` prefix included; strip it yourself | `{"user_id","is_anonymous","metadata"}`                                 |
| `handle_message` (sync mode) | `{"auth":{"user_id","instance_id","metadata"},"input_type","data"(base64)}`                                               | `{"output_type","data"(base64)}` — empty `output_type` = no reply frame |
| `on_new_connection`          | `{"auth"}`                                                                                                                | any 2xx admits; non-2xx rejects                                         |

Failure semantics (all verified by tests):

- **Fail closed:** any failure of `inspect_token` / `on_new_connection`
  (timeout, 5xx, transport error, open breaker) rejects the client.
  `handle_message` failures become an error frame to the client.
- **Deliberate rejection:** respond 4xx with `{"error":"your message"}` — that
  message is surfaced to the end client verbatim. Everything else is
  sanitized: end clients only ever see generic strings
  (`authentication failed` / `connection rejected` / `upstream error`); the
  full detail (URL, body) goes to the server log only.
- A 200 `inspect_token` response with empty `user_id` and
  `is_anonymous:false` is treated as a failure (fail closed).
- **Circuit breaker:** `BREAKER.THRESHOLD` (default 5) consecutive
  transport/5xx failures open it for `BREAKER.COOLDOWN` (default 10s); while
  open, sync callbacks fast-fail without hitting your backend. 4xx
  responses count as backend-alive (they close/reset the breaker).
- **Bounded retry on sync callbacks:** transport errors and 5xx responses are
  retried up to `SYNC_RETRY.MAX` attempts total (default `1` = no retry,
  since a client is waiting), waiting `SYNC_RETRY.BACKOFF` (default `100ms`)
  between attempts; 4xx responses are **never** retried (they're a
  deliberate rejection, see below). All attempts for one callback reuse the
  same `meta.id`, so dedupe on it if your backend can observe partial
  retries. Worst case, a client waits roughly
  `SYNC_RETRY.MAX × HANDLE_MESSAGE.TIMEOUT/SYNC_TIMEOUT + (SYNC_RETRY.MAX - 1) × SYNC_RETRY.BACKOFF`.

**JWT mode instead of `inspect_token`:** set `AUTH.MODE: jwt` and pipewave
verifies tokens locally (JWKS or static PEM; algorithms restricted to
RS*/ES*/EdDSA; `exp` required) — no callback round-trip per connection, and
the `inspect_token` event is never sent. Note JWT mode never produces
anonymous connections.

### 6.2 `handle_message` modes

|                      | `sync` (default)                          | `forward`                                      | `disabled`            |
| -------------------- | ----------------------------------------- | ---------------------------------------------- | --------------------- |
| Delivery             | blocking POST, response = reply frame     | async `message_received` event (at-least-once) | dropped               |
| Client latency       | +1 HTTP round-trip                        | none added                                     | none                  |
| Reply                | automatic (`output_type`/`data`)          | your backend pushes via REST when/if it wants  | none                  |
| Ordering / guarantee | per-connection order, exactly one attempt | retries may reorder; dedupe on `meta.id`       | —                     |
| Fit                  | RPC-style flows the client awaits         | telemetry, chat-style ingest                   | server-push-only apps |

### 6.3 Class 2 — asynchronous events (fire-and-forget, retried)

`on_new_connection_established`, `on_close_connection`, `on_read_error`,
`on_write_error` (`data`: `{"auth"}` or `{"auth","error"}`), and
`message_received` in forward mode. Respond any 2xx to acknowledge.

Delivery is at-least-once from an **in-memory** queue: backoff
`1s, 5s, 30s, 2m, 10m` (last repeats, configurable via `CALLBACKS.ASYNC_BACKOFF`)
up to `ASYNC_RETRY_MAX` attempts, then dropped with a warning log. Events are
lost on container crash — by design for v1; presence truth is always
`CheckOnline`/`GetUserSessions`, not events.

### 6.4 Backend health & shutdown

pipewave can actively probe your backend's health and react when it's down,
instead of only discovering trouble via failed sync callbacks.

- **Ping event:** when `CALLBACKS.PING.ENABLED: true`, pipewave POSTs a
  `event_type: "ping"` envelope (`data: {}`) to your callback endpoint —
  respond with any 2xx and an empty (or ignored) body.
- **Boot-check:** whenever ping is enabled, pipewave always pings once at
  startup before serving traffic. Failure is fatal — the server does not
  start. This is not a separate config knob; it's implied by
  `PING.ENABLED`.
- **Runtime ticker:** after boot, pipewave pings every `PING.INTERVAL`
  (default `30s`) with a `PING.TIMEOUT` (default `3s`) deadline per attempt.
  `PING.FAIL_THRESHOLD` (default `3`) consecutive failures marks the backend
  unhealthy; a subsequent success marks it healthy again.
- **`CALLBACKS.UNHEALTHY_ACTION`** decides what happens when the backend is
  marked unhealthy (by the ping ticker or by `BREAKER_OPEN_SHUTDOWN` below):
  - `shutdown` — pipewave triggers the same graceful shutdown path used for
    SIGINT/SIGTERM (HTTP servers drain, `pw.Shutdown()`, async queue flush),
    then exits with status `1`.
  - `log-only` (default) — pipewave only logs the transition; the process
    keeps running, but `GET /healthz` starts returning `503` until the
    backend recovers.
- **`CALLBACKS.BREAKER_OPEN_SHUTDOWN`** (default `0s`, disabled): if set
  `> 0`, the circuit breaker staying continuously open for at least that long
  also marks the backend unhealthy and feeds into `UNHEALTHY_ACTION` above —
  useful as a fallback when ping is disabled but repeated sync-callback
  failures indicate the backend is gone.

---

## 7. Admin REST API (your backend → pipewave)

Admin port, JSON, `Authorization: Bearer <api-key>` (exact scheme required).
Message payloads are **base64** in JSON (they are opaque bytes to pipewave).
Errors: `{"error":{"code":"<aerror code>","message":"..."}}` with the
matching HTTP status.

| Method | Route                                      | Body / result                                                                                                                                 |
| ------ | ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------- |
| POST   | `/api/v1/messages/session`                 | `{user_id, instance_id, msg_type, payload, ack_timeout_ms?}` → `{"sent":true}`; with `ack_timeout_ms` waits for client ack → `{"acked":bool}` |
| POST   | `/api/v1/messages/user`                    | `{user_id, msg_type, payload, ack_timeout_ms?}` — all sessions of the user                                                                    |
| POST   | `/api/v1/messages/users`                   | `{user_ids[], msg_type, payload}` — one publish, many users                                                                                   |
| POST   | `/api/v1/messages/broadcast`               | `{target:"all"\|"authenticated"\|"anonymous", msg_type, payload, instance_ids?[]}` (anonymous + no `instance_ids` = all anonymous)            |
| DELETE | `/api/v1/sessions/{user_id}/{instance_id}` | force-disconnect one session                                                                                                                  |
| DELETE | `/api/v1/sessions/{user_id}`               | force-disconnect all sessions of a user                                                                                                       |
| GET    | `/api/v1/sessions/{user_id}`               | `{"sessions":[{instance_id, holder_id, connection_type, status, connected_at}]}`                                                              |
| GET    | `/api/v1/presence/{user_id}`               | `{"online":bool}`                                                                                                                             |
| POST   | `/api/v1/presence/batch`                   | `{user_ids[]}` → `{"results":{id:bool}}`                                                                                                      |
| POST   | `/api/v1/maintenance/cleanup`              | `{"ok":true}`                                                                                                                                 |
| GET    | `/api/v1/monitoring/connections`           | `{"inside":{anonymous_connections,user_connections,total_users},"total":n}`                                                                   |
| GET    | `/api/v1/monitoring/worker-pool`           | `{"length","capacity","dropped"}`                                                                                                             |
| GET    | `/api/v1/webhook/public-key`               | `{"alg":"Ed25519","public_key_in_base64"}`                                                                                                    |
| GET    | `/healthz` (no auth)                       | 200 `{"healthy":true}` / 503                                                                                                                  |

Request bodies are decoded strictly (`DisallowUnknownFields`) — unknown JSON
fields return 400.

---

## 8. Running it

Local (see the root README for the full smoke-test script):

```bash
docker compose up -d valkey postgres
go run ./examples/rest-backend -addr :9000            # your stand-in backend
go run ./cmd/pipewave-server -config server-config.example.yaml
```

Container: `docker build -t pipewave-server:dev .` — multi-stage, distroless
`nonroot`, `WORKDIR /home/nonroot` (where the signing key is written; the
compose service mounts a named volume there so the key survives recreation).
A ready `pipewave-server` service exists in
[`docker-compose.yml`](../docker-compose.yml); note its comments about the
intentional duplicated `-config` flag and `host.docker.internal` on native
Linux.

Operational notes:

- `AUTO_MIGRATION: true` (or `pw.RunMigration()` at boot, which the binary
  always calls) creates tables; boot is fatal on migration failure.
- Rotating the signing key = replacing the key file (or volume) and
  re-fetching the public key in receivers.
- Deferred past v1 (by design, see spec): Prometheus `/metrics`, single-port
  mode, durable (Valkey-backed) async retry queue.
