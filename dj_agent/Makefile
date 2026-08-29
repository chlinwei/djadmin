# dj-agent 唯一认可的构建入口：所有目标强制 CGO_ENABLED=0，禁止直接裸跑 go build/go test。
export CGO_ENABLED := 0

GOOS ?= linux
GOARCH ?= amd64
BIN ?= bin/dj-agent

.PHONY: build test vet all

all: vet test build

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -buildvcs=false -o $(BIN) ./cmd/agent

test:
	go test ./...

vet:
	go vet ./...
