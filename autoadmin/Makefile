# autoadmin 的构建、测试和检查统一禁用 CGO，确保产物不依赖系统 C 运行库。
export CGO_ENABLED := 0

GOOS ?= linux
GOARCH ?= amd64
BIN ?= bin/autoadmin
PKG ?= ./...
TESTFLAGS ?=

# 版本注入：make build VERSION=v1.2.3（缺省用 git describe，仓库不可用则 dev）。
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X autoadmin/internal/buildinfo.Version=$(VERSION)

.PHONY: all build test vet fmt generate

all: vet test build

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -buildvcs=false -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/autoadmin

test:
	go test $(TESTFLAGS) $(PKG)

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

generate:
	go tool sqlc generate