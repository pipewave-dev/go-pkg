# Pipewave Server: REST API + Webhook Containerization

**Date:** 2026-07-17
**Status:** Approved (2026-07-17)

---

## Problem

Today pipewave can only be used as an embedded Go package: the host app calls
`pipewave.New(...)`, mounts `pw.Mux()` into its own HTTP server, injects
`types.Fns` (Go interfaces) for auth/message handling, and calls
`ExportedServices` methods directly (see `playground/` for the reference
integration).

This limits adoption to Go services. Teams running Node/Python/PHP/Java — or
teams that simply want a managed realtime component — cannot use pipewave
without writing a Go host themselves.

## Goals

- Ship pipewave as a standalone container (`pipewave-server`) that wraps the
  existing core untouched.
- Expose every `ExportedServices` method (plus `Monitoring`) as a REST API.
- Replace the injected `types.Fns` Go hooks with HTTP callbacks to the
  application backend (webhooks), with signed payloads.
- Keep the existing client-facing WebSocket/long-polling endpoints exactly as
  they are (`/issue-tmp-token`, `/gw`, `/lp`, `/lp-send`).
- The embedded-package mode keeps working unchanged; the container is an
  additional delivery mode built on top of the same public surface.
- Expose the same admin REST API to Go embedders via
  `restapi.NewAdminMux(pw, cfg)`, so embedded mode gets the HTTP surface for
  free.

## Non-Goals

- No changes to the core (`core/`, `provider/`, `export/`) beyond what a normal
  embedder could do. The server is strictly a wrapper.
- No client SDK work (JS/mobile clients already speak the WS protocol).
- No multi-tenant support in v1 — one container (fleet) serves one application
  backend.
- No gRPC surface in v1 (see Alternatives).

---

## Current surface being wrapped

**Methods → REST** (`core/delivery/module.go`):
`ExportedServices` — SendToSession, SendToUser, SendToUsers, SendToAll,
SendToAnonymous, SendToAuthenticated, SendToSessionWithAck, SendToUserWithAck,
DisconnectSession, DisconnectUser, CheckOnline, CheckOnlineMultiple,
GetUserSessions, CleanUp — plus `Monitoring` (InsideActiveConnection,
TotalActiveConnection, WorkerPoolStats) and `IsHealthy`.

**Hooks → callbacks** (`export/types/fns.go`):

| Hook                           | Shape                                            | Nature                                                             |
| ------------------------------ | ------------------------------------------------ | ------------------------------------------------------------------ |
| `InspectToken`                 | token → (userID, isAnonymous, metadata, err)     | **synchronous** — server needs the result to accept the connection |
| `HandleMessage`                | (auth, inputType, data) → (outputType, res, err) | **synchronous** — result is the WS reply frame                     |
| `OnNewConnection`              | (auth) → err                                     | **synchronous-ish** — returning err rejects the connection         |
| `OnCloseConnection`            | (auth)                                           | fire-and-forget                                                    |
| `OnReadError` / `OnWriteError` | (auth, err)                                      | fire-and-forget                                                    |

This split is the central design constraint: not every hook can become an
async webhook of the `callbackwebhook` style. The design therefore defines two
callback classes (see below).

---

## Approaches considered

### A. `cmd/pipewave-server` in this repo (recommended)

A new `cmd/pipewave-server/` main package plus a `server/` (or
`export/restwrap/`) package containing the REST handlers, webhook dispatcher,
and callback-backed `Fns` implementations. Built and published as a Docker
image from this repo.

- **Pros:** wrapper compiles against the exact core version it wraps — no
  version-skew; reuses existing config provider, aerror, otel, healthy
  provider; one repo to release.
- **Cons:** repo gains a binary + Dockerfile; slightly larger CI.

### B. Separate repo (`pipewave-server`) importing `go-pkg`

- **Pros:** keeps this repo a pure library.
- **Cons:** every core change needs a lockstep bump in the second repo; the
  wrapper needs nothing private, but drift between "library capability" and
  "server exposure" becomes an ongoing tax. Easy to extract later if ever
  needed.

### C. gRPC + streaming instead of REST + webhooks

Bidirectional gRPC stream would solve the synchronous-hook problem elegantly
(HandleMessage as a stream RPC), but it contradicts the stated goal (REST +
webhook like the niarb example), raises the integration bar for non-Go
backends, and complicates load-balancing.

**Recommendation: A.** Same repo, thin wrapper, REST + webhooks. C can be
revisited later as an alternative transport if sync-webhook latency proves
painful for high-throughput `HandleMessage` users.

---

## Architecture

```
                    ┌────────────────────────────────────────┐
 WS clients ───────►│  pipewave-server container             │
  /gw /lp ...       │                                        │
                    │  ┌──────────┐    ┌──────────────────┐  │        App backend
 App backend ──────►│  │ pw.Mux() │    │ REST API /api/v1 │  │  ──►  POST /pipewave/callback
  REST calls        │  └────┬─────┘    └────────┬─────────┘  │        (sync + async callbacks,
                    │       │    pipewave core  │            │         signed)
                    │  ┌────▼────────────────── ▼─────────┐  │
                    │  │ ModuleDelivery (unchanged)       │  │
                    │  └──────────────────────────────────┘  │
                    └────────────────────────────────────────┘
                         Valkey (pubsub/queue)  +  Postgres/DynamoDB
```

Two HTTP listeners (separate ports) so operators can network-isolate them:

- **Client port** (e.g. `:8080`): mounts `pw.Mux()` — public, reached by end
  users' browsers/apps.
- **Admin port** (e.g. `:8081`): `/api/v1/...` REST API + `/healthz` —
  private, reached only by the app backend / orchestrator. (A Prometheus
  `/metrics` endpoint is deferred past v1; drop events are observable via
  warning logs.)

A single-port mode (path-prefix split, admin auth still enforced) is
*deferred past v1* — Go embedders already get the equivalent by mounting
`NewAdminMux` into their own server; the container always runs two listeners.

### Components

1. **`server/restapi`** — HTTP handlers translating REST calls into
   `ExportedServices` / `Monitoring` calls. Stateless.
2. **`server/callback`** — outbound HTTP client implementing `types.Fns` by
   calling the app backend. Contains the signer, retry queue for async events,
   and circuit-breaker for sync calls.
3. **`server/authn`** — admin-API authentication (static API keys v1) and the
   optional built-in JWT validator for `InspectToken` (see below).
4. **`cmd/pipewave-server`** — wiring: load config (reuse
   `configprovider.FromYaml` + env overrides), `pipewave.New`, `RunMigration`,
   start listeners, graceful shutdown on SIGTERM (`pw.Shutdown()`).

---

## Callback design (the hooks)

Two classes, one shared envelope.

### Shared envelope

Adopted from the niarb `callbackwebhook` example:

```json
POST {callback_base_url}/{path}
X-Pipewave-Signature: <base64 signature over raw body>

{
  "data": { ...event-specific payload... },
  "meta": {
    "sent_at": 1789617000123,
    "id": "cb_01J9XYZ...",          // idempotency key (ULID)
    "event_type": "on_close_connection"
  }
}
```

- **Signing:** Ed25519 over the raw request body; signature in the
  `X-Pipewave-Signature` header (never in the body). Key pair generated at
  first boot and persisted (or supplied via config/secret). Public key exposed
  at `GET /api/v1/webhook/public-key` → `{ "alg": "Ed25519", "public_key_in_base64": "..." }`,
  mirroring the example's `PublicKeyVerifier`. Receivers must also reject
  `meta.sent_at` older than a configurable tolerance (default 5 min) to block
  replay.
- **Idempotency:** `meta.id` is unique per delivery attempt _group_ (retries
  reuse the same id) so receivers can dedupe.

### Class 1 — synchronous invocations (request/response)

Used for `inspect_token`, `handle_message`, `on_new_connection`. The server
POSTs and **blocks on the HTTP response**; the response body (also
JSON) is the hook's return value.

| event_type          | data (request)                                                    | expected response body                                                            |
| ------------------- | ----------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| `inspect_token`     | `{token, headers}`                                                | `{user_id, is_anonymous, metadata}` or `{error}`                                  |
| `handle_message`    | `{auth:{user_id,instance_id,metadata}, input_type, data(base64)}` | `{output_type, data(base64)}` or `{error}` — empty `output_type` = no reply frame |
| `on_new_connection` | `{auth}`                                                          | `200` accepts; `4xx {error}` rejects the connection                               |

Operational rules:

- Per-class timeout (defaults: inspect_token 3s, handle_message 5s,
  on_new_connection 3s). On timeout/5xx: `inspect_token` and
  `on_new_connection` **fail closed** (reject); `handle_message` returns an
  error frame to the client. No automatic retry — the WS client is waiting.
- Circuit breaker: consecutive failures trip the breaker and fast-fail,
  protecting the app backend during outages.
- **Escape hatch for `inspect_token` (recommended default for production):**
  a built-in JWT validation mode — configure JWKS URL / static public key +
  claim mapping (`sub` → user_id, configurable metadata claims). This removes
  one network round-trip per connection; the webhook mode remains for custom
  token schemes. Exactly one of the two modes must be configured.
### `handle_message`: `sync` mode vs `forward` mode

Two delivery modes for client → server messages, selected by config.

**`sync` mode (default).** Every client WS message triggers one blocking HTTP
POST to the app backend; the HTTP response body becomes the WS reply frame.
This preserves the exact request/response semantics of the embedded
`HandleMessage` hook: the client gets its reply frame (with `ResponseToId`
auto-set by the SDK) and errors surface immediately as error frames.

**`forward` mode.** The message is delivered as a Class-2 async event
(`event_type: message_received`, at-least-once with retry) and pipewave sends
**no reply frame**. If the app backend wants to respond, it does so on its own
schedule by calling `POST /api/v1/messages/session` (or `/user`) — from the
backend's point of view every reply is just a normal server push.
Request/response correlation, if needed, must be carried inside the app's own
payload (e.g. echo back a client-generated request id).

| | `sync` | `forward` |
|---|---|---|
| Semantics | request/response — reply frame per message | fire-and-forget ingest — reply (if any) is a separate push |
| Latency felt by WS client | + 1 HTTP round-trip to backend per message (client blocks on the reply frame) | none added — message is queued and the WS loop moves on |
| Backend slow/down | client waits up to `timeout`, then gets an error frame; circuit breaker fast-fails | client unaffected; events retry with backoff (may arrive late or, after max retries / container crash, be lost) |
| Delivery guarantee | exactly the one attempt — no retry (client is waiting) | at-least-once (backend must dedupe on `meta.id`) |
| Ordering | per-connection order preserved (one message at a time) | best-effort — retries can reorder |
| Backend implementation | must answer within timeout, response schema fixed | just an event consumer + normal REST send calls |
| Good for | commands, RPC-style interactions, anything the client awaits (`REQ_RES`-style flows in `playground/handlemsg.go`) | telemetry, typing indicators, high-throughput ingest, chat-style flows where replies are pushes anyway |

**Recommendation:** ship both, document `sync` as the default. It matches the
mental model of the existing embedded hook, and correctness (a client waiting
on a reply) should be the out-of-the-box behavior; teams with throughput needs
opt into `forward` deliberately, accepting at-least-once + no reply frames.

### Class 2 — asynchronous events (fire-and-forget with retry)

Used for `on_close_connection`, `on_read_error`, `on_write_error`,
`message_received` (when `handle_message` runs in `forward` mode), and (for
observability symmetry) `on_new_connection_established` emitted after a
connection is accepted.

- Delivered with at-least-once semantics: in-memory queue with exponential
  backoff (e.g. 1s/5s/30s/2m/10m, max N attempts), then dropped with a warning
  log + metric. **Decided:** loss on container crash is acceptable for v1 —
  these events are advisory (the source of truth for presence is
  `CheckOnline`/`GetUserSessions`). If durable delivery is later required, the
  existing queue provider (Valkey) can back the retry queue.
- All Class-2 events go to a single configured URL; receivers switch on
  `meta.event_type` (same pattern as the niarb example's constants).

---

## REST API mapping

All under `/api/v1`, JSON bodies, authenticated via `Authorization: Bearer <api-key>`
(static keys in config v1; multiple keys allowed for rotation). Errors use the
existing `aerror` code + i18n message serialized as
`{"error": {"code": ..., "message": ...}}`.

| Method | Route                                      | Wraps                                                                                                                                                               |
| ------ | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| POST   | `/api/v1/messages/session`                 | SendToSession — body `{user_id, instance_id, msg_type, payload(base64), ack_timeout_ms?}`; if `ack_timeout_ms` set → SendToSessionWithAck, responds `{acked}`       |
| POST   | `/api/v1/messages/user`                    | SendToUser / SendToUserWithAck (same `ack_timeout_ms` convention)                                                                                                   |
| POST   | `/api/v1/messages/users`                   | SendToUsers — `{user_ids[], msg_type, payload}`                                                                                                                     |
| POST   | `/api/v1/messages/broadcast`               | SendToAll / SendToAuthenticated / SendToAnonymous — `{target: "all"\|"authenticated"\|"anonymous", msg_type, payload, instance_ids?[]}` (mirrors `BroadcastTarget`) |
| DELETE | `/api/v1/sessions/{user_id}/{instance_id}` | DisconnectSession                                                                                                                                                   |
| DELETE | `/api/v1/sessions/{user_id}`               | DisconnectUser                                                                                                                                                      |
| GET    | `/api/v1/presence/{user_id}`               | CheckOnline → `{online: bool}`                                                                                                                                      |
| POST   | `/api/v1/presence/batch`                   | CheckOnlineMultiple — `{user_ids[]}` → `{results: {id: bool}}`                                                                                                      |
| GET    | `/api/v1/sessions/{user_id}`               | GetUserSessions → `[{instance_id, holder_id, connection_type, status, connected_at}]`                                                                               |
| POST   | `/api/v1/maintenance/cleanup`              | CleanUp                                                                                                                                                             |
| GET    | `/api/v1/monitoring/connections`           | InsideActiveConnection + TotalActiveConnection                                                                                                                      |
| GET    | `/api/v1/monitoring/worker-pool`           | WorkerPoolStats                                                                                                                                                     |
| GET    | `/api/v1/webhook/public-key`               | Ed25519 public key for signature verification                                                                                                                       |
| GET    | `/healthz` (admin port, unauthenticated)   | IsHealthy — liveness/readiness                                                                                                                                      |

Payloads are base64 in JSON because `payload []byte` is opaque to pipewave
(apps may send binary frames).

`OnNewRegister`/`OnCloseRegister` (the in-process registration API) are **not**
exposed over REST — their function is fully covered by the Class-1/Class-2
callbacks above.

**Embedded mode (in scope):** the same handler set is exposed to Go embedders
as `restapi.NewAdminMux(pw, cfg) *http.ServeMux`, built purely from the public
`ModuleDelivery` surface (`Services()`, `Monitoring()`, `IsHealthy()`), so
embedders can mount the admin API into their own server with auth enforced via
the configured API keys. The container mounts the same mux on its admin
listener — one implementation, two consumers. A free function (rather than a
new `ModuleDelivery` method) keeps the "no core changes" non-goal intact.

---

## Configuration

Extends the existing YAML config (`configprovider.FromYaml`) with a `server`
section; the core `EnvType` section is unchanged:

```yaml
server:
    client_addr: ":8080"
    admin_addr: ":8081"
    api_keys: ["${PIPEWAVE_API_KEY}"]
    auth:
        mode: jwt # jwt | webhook
        jwt:
            jwks_url: https://app.example.com/.well-known/jwks.json
            user_id_claim: sub
            metadata_claims: [role, tenant]
    callbacks:
        base_url: https://app.example.com/pipewave/callback
        signing_key_file: /secrets/webhook_ed25519.key # generated if absent
        handle_message:
            mode: sync # sync | forward | disabled
            timeout: 5s
        sync_timeout: 3s
        async_retry_max: 6
```

Every field overridable via env vars for 12-factor deployment.

---

## Error handling summary

- REST: `aerror` → HTTP status mapping (validation → 400, not-found → 404,
  internal → 500) with the aerror code preserved in the body.
- Sync callbacks: timeout/5xx → fail closed (auth/new-connection) or error
  frame (handle_message); circuit breaker trips after consecutive failures.
- Async callbacks: retry with backoff, then drop + warning log (a
  `pipewave_webhook_dropped_total` metric comes with the deferred `/metrics`
  endpoint).
- Startup: `RunMigration()` before listening; fatal on failure.
- Shutdown: stop accepting, `pw.Shutdown()` (drains via existing graceful
  shutdown work), flush async retry queue best-effort within a deadline.

## Testing

- Handler-level unit tests with a fake `ExportedServices` (table-driven,
  per repo convention).
- Callback dispatcher tests against `httptest.Server`: signature correctness,
  timeout → fail-closed, retry/backoff schedule, idempotent `meta.id` across
  retries.
- One end-to-end smoke test reusing the playground docker-compose stack
  (Valkey + Postgres): boot the server, connect a WS client, drive
  `handle_message` through a stub backend, send via REST, assert delivery.
- Signature verification golden test so a receiver implementation in another
  language can be validated against the same vectors.

## Rollout / packaging

- `Dockerfile` (multi-stage, distroless) + entry in `docker-compose.yml` for
  local dev.
- Versioning: server image version = module version (single repo, approach A).
- Docs: an `examples/rest-backend/` stub receiver (any language) showing
  signature verification + the three sync endpoints.

## Decisions (2026-07-17 review)

1. **Class-2 durability:** loss of Class-2 events on container crash is
   acceptable for v1 — in-memory retry queue only; Valkey-backed durable
   queue stays a future option.
2. **`handle_message` default mode:** both modes ship; see the
   "`sync` mode vs `forward` mode" section above for the full comparison.
   Default is `sync` (matches the embedded hook's request/response
   semantics); `forward` is an explicit opt-in for high-throughput,
   push-style apps.
3. **Admin API auth:** static API keys are sufficient for v1 — no mTLS or
   HMAC request signing required before launch.
4. **Embedded admin API:** in scope — exposed as
   `restapi.NewAdminMux(pw, cfg)` (see REST API mapping section).
