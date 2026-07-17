# syntax=docker/dockerfile:1
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pipewave-server ./cmd/pipewave-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/pipewave-server /pipewave-server
# Mount your config at /etc/pipewave/config.yaml (see server-config.example.yaml)
ENTRYPOINT ["/pipewave-server", "-config", "/etc/pipewave/config.yaml"]
