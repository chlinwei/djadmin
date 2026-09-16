# dj-agent 唯一认可的构建入口：所有目标强制 CGO_ENABLED=0，禁止直接裸跑 go build/go test。
export CGO_ENABLED := 0

GOOS ?= linux
GOARCH ?= amd64
BIN ?= bin/dj-agent

# 版本注入：make build VERSION=v1.2.3（缺省用 git describe，仓库不可用则 dev）。
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X github.com/chlinwei/djadmin/dj_agent/internal/buildinfo.Version=$(VERSION)

# 巡检执行器：唯一为 OPA（Rego 策略），以 Go 库（github.com/open-policy-agent/opa/v1/rego）
# 进程内求值，不需要内嵌二进制；版本随 go.mod 管理。

.PHONY: build test vet all

all: vet test build

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -buildvcs=false -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/agent

test:
	go test ./...

vet:
	go vet ./...
