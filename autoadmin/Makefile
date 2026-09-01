# autoadmin 的构建、测试和检查统一禁用 CGO，确保产物不依赖系统 C 运行库。
export CGO_ENABLED := 0

GOOS ?= linux
GOARCH ?= amd64
BIN ?= bin/autoadmin
PKG ?= ./...
TESTFLAGS ?=

.PHONY: all build test vet fmt generate

all: vet test build

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -buildvcs=false -trimpath -o $(BIN) ./cmd/autoadmin

test:
	go test $(TESTFLAGS) $(PKG)

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

generate:
	go tool sqlc generate