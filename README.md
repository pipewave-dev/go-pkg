# pipewave/gopkg

> **Real-time WebSocket, Simplified.**

`pipewave/gopkg` is the official Go SDK for [Pipewave](https://pipewave.dev) — a production-ready, embeddable library for real-time WebSocket communication. Drop it into any Go HTTP server and get authenticated connections, automatic long-polling fallback, and horizontal scaling out of the box.

**Website & full documentation → [https://pipewave.dev](https://pipewave.dev)**

---

## Why Pipewave?

Building real-time features is hard: token authentication over WebSocket, multi-tab/multi-device fanout, scaling across containers, detecting stale connections — all before you write a single line of business logic.

Pipewave handles all of that as a single importable module, so you focus on what your app does, not on the plumbing.

---

## Features

| | |
|---|---|
| **Two-stage token exchange** | Secure handshake — no JWTs in URLs |
| **Long-polling fallback** | Transparent fallback when WebSocket is unavailable |
| **User-level broadcasting** | Send to a user ID and reach all their sessions (multi-tab, multi-device) |
| **Horizontal scaling** | Pub/sub across containers — no sticky sessions required |
| **High-performance I/O** | [gobwas/ws](https://github.com/gobwas/ws) + netpoll reactor (~0.2 KB/conn vs ~8 KB with gorilla/websocket) |
| **Pluggable repositories** | DynamoDB and PostgreSQL included; bring your own via interfaces |
| **Built-in heartbeat** | Online/offline status per user and session |
| **Rate limiting** | Per-connection token bucket, shared across all sessions of the same user |
| **Message deduplication** | Client-supplied message IDs prevent double-processing |
| **Worker pool** | Bounded concurrency for async ops; prevents goroutine explosion under load |
| **OpenTelemetry** | Distributed tracing via OTLP gRPC/HTTP, stdout, file, or discard exporters |
| **Wire DI** | Compile-time dependency injection via [google/wire](https://github.com/google/wire) |

---

## Requirements

- Go 1.25+
- **Valkey / Redis** — caching and pub/sub broadcasting
- **DynamoDB or PostgreSQL** — persistent storage for active connections

Start local infrastructure with Docker Compose:

```bash
docker compose up -d
```

This starts:
- **Valkey** on port `29100`
- **DynamoDB Local** on port `29101`

---

## Installation

```bash
go get github.com/pipewave-dev/go-pkg
```

---

## Quick Start

### 1. Load configuration

```go
import pipewave "github.com/pipewave-dev/go-pkg"

config := pipewave.ConfigFromYaml(
    []string{"config.yaml"},
    pipewave.FunctionStore{
        InspectToken:  inspectToken,
        HandleMessage: handleMessage,
    },
)
```

### 2. Initialize Pipewave

```go
pw := pipewave.NewPipewave(pipewave.PipewaveConfig{
    ConfigStore: config,
    // RepositoryFactory -> DynamoDB or PostgreSQL (default: PostgreSQL)
    // QueueFactory      -> Valkey (default)
    // SlogIns           -> your slog instance (default: slog.Default())
})
defer pw.Shutdown()
```

### 3. Serve HTTP

```go
http.ListenAndServe(":8080", pw.Mux())
```

### 4. Send messages programmatically

```go
svc := pw.Services().Websocket()

// Broadcast to all sessions of a user (multi-tab, multi-device)
svc.SendToUser(ctx, "user-id", "notification", payload)

// Send to one specific session
svc.SendToSession(ctx, "user-id", "session-id", "msg-type", payload)

// Send to anonymous connections
svc.SendToAnonymous(ctx, "msg-type", payload, false, sessionIDs)
```

### Your logic functions

```go
// InspectToken — called on every connection to authenticate the client
func inspectToken(ctx context.Context, token string) (configprovider.WebsocketAuth, error) {
    userID, err := validateJWT(ctx, strings.TrimPrefix(token, "Bearer "))
    if err != nil {
        return configprovider.WebsocketAuth{}, err
    }
    return configprovider.UserAuth(userID), nil
}

// HandleMessage — called when a client sends a message
func handleMessage(
    ctx context.Context,
    auth voAuth.Auth,
    inputType string,
    data []byte,
) (outputType string, res []byte, err error) {
    switch inputType {
    case "echo":
        return "echo-response", []byte("rev" + string(data)), nil
    default:
        return "", nil, nil // empty outputType means no reply to client
    }
}
```

---

## Documentation

Full guides, configuration reference, and API docs are at **[https://pipewave.dev](https://pipewave.dev)**.

---

## Contributing

Contributions are welcome! Please open an issue or submit a pull request on GitHub.

- Fork the repository and create your branch from `main`
- Ensure your code passes existing tests and linting: `go test ./...` · `golangci-lint run`
- Write clear commit messages
- Open a pull request describing your changes

---

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). Please report unacceptable behavior to the project maintainers.

---

## License

See [LICENSE](LICENSE) for details.
