# dj-agent 唯一认可的构建入口：所有目标强制 CGO_ENABLED=0，禁止直接裸跑 go build/go test。
export CGO_ENABLED := 0

GOOS ?= linux
GOARCH ?= amd64
BIN ?= bin/dj-agent

# 内嵌 goss（巡检 Goss 执行器）：
# - resources/goss/ 下存放官方发布版二进制（见 resources/goss/VERSION），升级时替换文件并更新 VERSION/SHA256SUMS。
# - 构建前按目标架构把对应二进制复制到 embed 目录，go:embed 只打包当前架构，避免体积翻倍。
GOSS_VERSION := $(shell cat resources/goss/VERSION)
GOSS_ARCH := $(if $(filter amd64,$(GOARCH)),amd64,$(if $(filter arm64,$(GOARCH)),arm64,unsupported))
GOSS_RESOURCE := resources/goss/goss-linux-$(GOSS_ARCH)
GOSS_EMBED_DIR := internal/executor/embedded/goss
GOSS_EMBED := $(GOSS_EMBED_DIR)/goss-linux-$(GOSS_ARCH)

.PHONY: build test vet all goss-verify clean-embed

all: vet test build

$(GOSS_EMBED): $(GOSS_RESOURCE)
	@mkdir -p $(GOSS_EMBED_DIR)
	@cp $(GOSS_RESOURCE) $@
	@echo "embedded goss $(GOSS_VERSION) ($(GOSS_ARCH))"

goss-verify: $(GOSS_RESOURCE)
	@cd resources/goss && sha256sum --check --quiet SHA256SUMS

build: goss-verify $(GOSS_EMBED)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -buildvcs=false -o $(BIN) ./cmd/agent

test: goss-verify $(GOSS_EMBED)
	go test ./...

vet:
	go vet ./...

clean-embed:
	@rm -f $(GOSS_EMBED_DIR)/goss-linux-*
