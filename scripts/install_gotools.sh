#!/bin/bash

please() {
  if command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    "$@"
  fi
}

require_min_version() {
  installed_version=$1
  min_version=$2

  if ! printf '%s\n%s\n' "$min_version" "$installed_version" | sort -V -C; then
    echo "Current version is $installed_version. Required version >= $min_version"
    return 1 #False
  else
    return 0 #True
  fi
}

lowercase() {
  echo "$1" | tr '[:upper:]' '[:lower:]'
}

uppercase() {
  echo "$1" | tr '[:lower:]' '[:upper:]'
}

install_aws() {
if ! command -v "aws" &> /dev/null; then
  # Install aws cli
  OS=$(lowercase $(uname -s))
  ARCH=$(lowercase $(uname -m))
  curl "https://awscli.amazonaws.com/awscli-exe-$OS-$ARCH.zip" -o "awscliv2.zip"
  please unzip -qq awscliv2.zip
  please ./aws/install
  please rm awscliv2.zip
  please rm -rf aws
  echo "✅ aws"
else
  echo "✅ aws"
fi
}

install_bufbuild_step() {
  BINPATH="/usr/local/bin" && \
  VERSION="1.50.0" && \
  please curl -sSL \
  "https://github.com/bufbuild/buf/releases/download/v${VERSION}/buf-$(uname -s)-$(uname -m)" \
  -o "${BINPATH}/buf" && \
  please chmod +x "${BINPATH}/buf"
}

install_bufbuild() {
if ! command -v "buf" &> /dev/null; then
  install_bufbuild_step
  echo "✅ bufbuild"

else
  installed_version=$(buf --version)
  min_version="1.50.0"

  if require_min_version "$installed_version" "$min_version"; then
    echo "✅ bufbuild"
  else
    echo "Upgrade buf to version $min_version"
    install_bufbuild_step
    echo "✅ bufbuild"
  fi
fi
}

install_nats_cli() {
if ! command -v "nats" &> /dev/null; then
  go install github.com/nats-io/natscli/nats@latest
  echo "✅ nats"
else
  echo "✅ nats"
fi
}

# Config private
go env -w GOPROXY=https://goproxy.io,https://proxy.golang.org,direct && \
go env -w GOPRIVATE=${GIT_URL}

# Tools for IDE
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.59.1
go install golang.org/x/tools/cmd/goimports@latest
go install golang.org/x/tools/gopls@latest
go install github.com/go-delve/delve/cmd/dlv@latest
go install github.com/cweill/gotests/gotests@latest
go install github.com/fatih/gomodifytags@latest
go install github.com/josharian/impl@latest

# Plugins for proto
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3


install_bufbuild
install_aws
install_nats_cli
