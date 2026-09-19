# autoadmin

The Go backend of djadmin (`autoadmin`). It serves the Vue frontend's existing API contract over the migrated MySQL schema; the data access layer also targets PostgreSQL as a second dialect (see [SQL_DESIGN](../docs/architecture/SQL_DESIGN.md)). The Django implementation has been fully retired and removed from the repository — historical docs live in [docs/archive/](../docs/archive/).

## Stack

- Go 1.27.0
- Gin v1.12.0
- sqlc — generation **must** use v1.30.0 (`go.mod` pins the tool at v1.31.1, which silently changes generated params; the Makefile guards the version). See SQL_DESIGN §5.1
- go-sql-driver/mysql v1.10.0 (default build)
- jackc/pgx v5 (`-tags postgres` build)
- gocron v2.22.0
- RabbitMQ 4.3.5 with amqp091-go v1.14.0
- go-ansible v2.4.1
- golang-migrate v4.19.1
- `log/slog` for structured logging

All builds and tests must use the Makefile. It exports `CGO_ENABLED=0`, and the command package contains a build guard that intentionally fails when CGO is enabled.

## Build and run

```bash
# ---- default build: MySQL (behaviour identical to before the dialect work) ----
make test                     # go test ./...
make vet                      # go vet ./...
make build                    # -> bin/autoadmin

# ---- PostgreSQL variant: queries use the PG artifacts, connection uses pgx ----
make build-postgres           # go build -tags postgres -> bin/autoadmin-postgres
make vet-postgres             # go vet -tags postgres ./...
go test -tags postgres ./...  # run the full suite for this variant too

# ---- SQL generation pipeline (order matters; see SQL_DESIGN §4.6) ----
make derive                   # db/queries/mysql -> db/queries/postgres (never edit the PG side)
make generate SQLC=~/go/bin/sqlc   # both sqlc artifacts; requires sqlc v1.30.0
make facade                   # rebuild the dialect facade (must run after make generate)

# ---- run (one binary, four roles) ----
./bin/autoadmin api           # HTTP :9000 + agent gRPC :9001 + 日志采集批量作业队列
./bin/autoadmin scheduler
./bin/autoadmin worker        # 通用作业队列（计划任务）
./bin/autoadmin migrate       # applies db/migrations/<dialect> up migrations
./bin/autoadmin migrate force 34   # 迁移失败后清脏标记（只改版本表，不动 schema）
./bin/autoadmin --version     # or -v
```

迁移在真库上失败时（例如某个 `ALTER` 被外键挡住）：golang-migrate 会把版本表写成
`(version=N, dirty=1)` 并从此拒绝执行。DDL 失败那一步通常没有落库，所以处理顺序是
**改好迁移文件 → `migrate force <失败前的版本号>` → 再 `migrate`**。`force` 只改版本号，
置错会让迁移链与库内实际结构错位，不确定时先核对 `schema_migrations` 与库内结构。

队列按"执行者需要什么"分成两条（见 `rabbitmq.Routes`）：`autoadmin.job.execute`
（不需要 agent 会话的作业：计划任务，worker 角色消费）与 `autoadmin.logcollect.execute`
（日志采集批量动作：要经 agent gRPC 会话在主机上执行，而会话只存在于 api 进程里，
因此**必须由 api 角色消费**，并发/预算由 `LOG_BATCH_*` 控制）。

Environment: the default build reads `MYSQL_DSN`; the `-tags postgres` build reads `POSTGRES_DSN` (pgx DSN, must carry `TimeZone=UTC`). `MIGRATION_DATABASE_URL` / `MIGRATION_SOURCE_URL` are used by the `migrate` role. Configuration is not read from dotenv files — export it (`set -a; . ./config.env; set +a`) or inject it from the deployment environment.

Note: `db/migrations/postgres/` holds the PostgreSQL translation of every MySQL migration (same file names and version numbers; see SQL_DESIGN §4.7). The `migrate` role registers the driver matching the build: `!postgres` → `migrate/v4/database/mysql`, `-tags postgres` → `migrate/v4/database/pgx/v5`. **The PG variant's `MIGRATION_DATABASE_URL` must use the `pgx5://` scheme** (golang-migrate dispatches by scheme), e.g. `pgx5://user:pass@host:5432/djadmin?sslmode=disable`; `MIGRATION_SOURCE_URL` then points at `file://db/migrations/postgres`.

**Both tags must be built and tested in CI**: the PostgreSQL adapters are hand-written, so a new divergence between the two artifacts only surfaces when the `postgres` tag is compiled (see SQL_DESIGN §4.8).

## Version

`internal/buildinfo` 定义 `Version`（源码默认 `dev`），构建时由 Makefile 经 `-ldflags -X` 注入。`VERSION` 缺省用 `git describe --tags --always --dirty`（git 不可用则 `dev`），也可显式指定：

```bash
make build VERSION=v1.2.3
```

版本出现在 `--version` 输出及各角色（api/scheduler/worker/migrate）启动日志中。裸 `make build` 时的取值规则与 dj-agent 一致：标签干净 → `v1.0.0`；有未提交改动追加 `-dirty`；无标签 → 短 commit。

Only the API bootstrap and infrastructure adapters are wired in the initial skeleton. Scheduler definitions, worker dispatch and migrations are intentionally enabled domain by domain after the schema baseline is generated.

## Layout

```text
cmd/autoadmin/                   process entry point and CGO guard
internal/api/                    Gin server, routing and response envelope
internal/app/                    role composition and lifecycle
internal/config/                 environment configuration
internal/modules/                Django-to-Go domain catalog
internal/platform/database/      database pool, query derivation, dialect facade and generated sqlc
  derive/                        query derivation + facade generator + drift tests (make derive / facade)
  generated/                     dialect facade (fixed path imported by app code; dialect_mysql.go /
                                 dialect_postgres.go are generated, *_adapters.go is hand-written)
  generated/mysql/               MySQL sqlc output (default build)
  generated/postgres/            PostgreSQL sqlc output (-tags postgres)
internal/messaging/rabbitmq/     durable topology, publisher and consumer
internal/scheduler/              gocron scheduler and message publication
internal/automation/ansible/     go-ansible adapter
sqlc.yaml                        sqlc config: both dialects (make generate emits both)
db/schema/<dialect>/             schema baseline consumed by sqlc, one per dialect
db/queries/mysql/                the single hand-maintained source of queries
db/queries/postgres/             derived from db/queries/mysql (do not edit by hand)
db/migrations/mysql/             post-baseline migrations (000001…000022, up/down per version)
db/migrations/postgres/          same 44 file names/versions, translated for PostgreSQL (hand-maintained)
docs/                            architecture, contracts and migration plan
```

The full SQL tree — which directory is hand-maintained, which is a generated artifact, and the required order of the `derive` / `generate` / `facade` steps — is in [SQL_DESIGN §4.6](../docs/architecture/SQL_DESIGN.md).

Use `config.example.env` as the local environment template and provide real credentials outside Git. The application does not load dotenv files itself.

See [Architecture](docs/ARCHITECTURE.md), [API contract](docs/API_CONTRACT.md), [business workflows](docs/BUSINESS_WORKFLOWS.md) and [error conventions](docs/ERROR_CONVENTIONS.md).

业务语义与错误约定仍以这两个为准：[Business workflows and state machines](docs/BUSINESS_WORKFLOWS.md)、[Error conventions](docs/ERROR_CONVENTIONS.md)。

迁移期的 Django 基线文档（API 深度分析 / 领域模型分析 / 模块映射 / Go 重写指南 / 开发计划）已随实现迁出归档到
[docs/archive/](../docs/archive/)（历史参考，不作为实现依据）。