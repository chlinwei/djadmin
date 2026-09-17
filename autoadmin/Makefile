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

.PHONY: all build test vet fmt generate derive facade build-postgres vet-postgres

all: vet test build

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -buildvcs=false -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/autoadmin

test:
	go test $(TESTFLAGS) $(PKG)

vet:
	go vet ./...

fmt:
	gofmt -w ./cmd ./internal

# sqlc 版本必须与仓库产物一致（v1.30.0）。go.mod 的 tool 指令已 pin 到 v1.30.0，
# 这里再校验一次作为兜底：v1.31.1 会把重复的 sqlc.arg 去重、改变生成结果并打断
# 调用方 —— 静默产出错误代码比报错更危险。需要临时用其它二进制可覆盖：
# make generate SQLC=~/go/bin/sqlc。详见 docs/architecture/SQL_DESIGN.md §5。
SQLC ?= go tool sqlc
SQLC_REQUIRED := v1.30.0

define require_sqlc
	@v=$$($(SQLC) version 2>/dev/null | head -1); \
	if [ "$$v" != "$(SQLC_REQUIRED)" ]; then \
		echo "sqlc 版本不符：当前 '$$v'，要求 $(SQLC_REQUIRED)。"; \
		echo "它会产生与仓库不一致的生成结果。请用：make generate SQLC=<v1.30.0 的二进制路径>"; \
		exit 1; \
	fi
endef

# sqlc.yaml 里有两个 block（mysql / postgresql），一次生成两侧产物。
generate:
	$(require_sqlc)
	$(SQLC) generate

# 从 db/queries/mysql 派生 db/queries/postgres（PG 查询集是产物，禁止手改）。
# 改了 MySQL 源没跑本目标的话，go test 会直接失败并指出首个差异行。
derive:
	go test ./internal/platform/database/derive -run TestDerivedQueriesMatchRepository -update -v

# 门面（dialect_mysql.go / dialect_postgres.go）是生成的产物：类型别名跟着 sqlc 产物变，
# 所以必须在 make generate 之后跑。一致性由 derive 包的 TestFacadeMatchesRepository 守住。
facade:
	go test ./internal/platform/database/derive -run TestFacadeMatchesRepository -update -v

# PostgreSQL 变体：查询与连接都走 PG 产物/驱动，需要 POSTGRES_DSN。
build-postgres:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -tags postgres -buildvcs=false -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)-postgres ./cmd/autoadmin

vet-postgres:
	go vet -tags postgres ./...
