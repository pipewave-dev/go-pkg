# pipewave-gopkg

**Pipewave** is a production-ready, embeddable Go library for real-time WebSocket communication. It provides authenticated WebSocket connections with automatic long-polling fallback and horizontal scaling via pub/sub — all packaged as a single importable module.

---

## Features

- **Two-stage token exchange authentication** — secure WebSocket handshake without exposing JWTs in URLs
- **Automatic long-polling fallback** — transparent fallback for environments where WebSocket is unavailable
- **User-level broadcasting** — send to a user ID and reach all their active sessions (multi-tab, multi-device)
- **Horizontal scaling** — pub/sub broadcasting across multiple containers with no sticky-session requirement
- **High-performance I/O** — [gobwas/ws](https://github.com/gobwas/ws) + netpoll event reactor (~0.2 KB per connection vs ~8 KB with gorilla/websocket)
- **Pluggable repositories** — DynamoDB and PostgreSQL implementations included; add your own via interfaces
- **Built-in heartbeat** — detect online/offline status per user and session
- **Rate limiting** — per-connection token bucket, shared across all sessions of the same user
- **Message deduplication** — client-supplied message IDs prevent double-processing
- **Worker pool** — bounded concurrency for async operations; prevents goroutine explosion under load
- **OpenTelemetry** — distributed tracing with OTLP gRPC/HTTP, stdout, file, or discard exporters
- **Wire dependency injection** — compile-time DI via [google/wire](https://github.com/google/wire)

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
	// RepositoryFactory -> DynamoDB (default) or Postgresql
	// QueueFactory      -> Currently only support Valkey (default)
	// SlogIns           -> Give your slog instance (default is slog.Default())
})
defer pw.Shutdown()
```

### 3. Serve HTTP

```go
http.ListenAndServe(":8080", pw.Mux())
```

### 4. Send messages programmatically

```go
services := pw.Services().Websocket()

// Send to all sessions of a user (multi-tab, multi-device)
services.SendToUser(ctx, "user-id", "notification", payload)

// Send to one specific session
services.SendToSession(ctx, "user-id", "session-id", "msg-type", payload)

// Send to anonymous connections
services.SendToAnonymous(ctx, "msg-type", payload, false, sessionIDs)
```

### Your logic functions

```go
// InspectToken: called on every connection to authenticate the client
func inspectToken(ctx context.Context, token string) (configprovider.WebsocketAuth, error) {
    userID, err := validateJWT(ctx, strings.TrimPrefix(token, "Bearer "))
    if err != nil {
        return configprovider.WebsocketAuth{}, err
    }
    return configprovider.UserAuth(userID), nil
}

// HandleMessage: called when a client sends a message
func handleMessage(
    ctx context.Context,
    auth voAuth.Auth,
    inputType string,
    data []byte,
) (outputType string, res []byte, err error) {
    switch inputType {
    case "echo":
        return "echo-response", []byte("rev"+string(data)), nil
    default:
        return "", nil, nil // When outputType == "", it won't send back to client
    }
}
```

## Documentation

Full documentation, guides, and API reference are available at **[https://pipewave.dev](https://pipewave.dev)**.

---

## Contributing

Contributions are welcome! Please open an issue or submit a pull request on GitHub.

- Fork the repository and create your branch from `main`
- Ensure your code passes existing tests and linting (`go test ./...`, `golangci-lint run`)
- Write clear commit messages
- Open a pull request describing your changes

---

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). By participating, you are expected to uphold this standard. Please report unacceptable behavior to the project maintainers.

---

## License

See [LICENSE](LICENSE) for details.
