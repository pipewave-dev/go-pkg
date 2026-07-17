# syntax=docker/dockerfile:1
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pipewave-server ./cmd/pipewave-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/pipewave-server /pipewave-server
# /home/nonroot is owned by uid 65532 (nonroot) in the base image, so the
# default relative SigningKeyFile path (webhook_ed25519.key) is writable here;
# without a WORKDIR it resolves against "/", which nonroot cannot write to.
# NOTE: one of the two config loaders used at startup also resolves the
# -config path relative to this WORKDIR (it strips any leading "/" from the
# flag value), so mount your config at BOTH /etc/pipewave/config.yaml and
# $WORKDIR/etc/pipewave/config.yaml, i.e. /home/nonroot/etc/pipewave/config.yaml
# (see docker-compose.yml).
WORKDIR /home/nonroot
ENTRYPOINT ["/pipewave-server", "-config", "/etc/pipewave/config.yaml"]
