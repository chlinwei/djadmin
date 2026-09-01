# autoadmin

Go rewrite of the djadmin backend. The project preserves the existing Vue frontend API contract and MySQL tables while domains are migrated incrementally.

## Stack

- Go 1.27.0
- Gin v1.12.0
- sqlc v1.31.1
- go-sql-driver/mysql v1.10.0
- gocron v2.22.0
- RabbitMQ 4.3.5 with amqp091-go v1.14.0
- go-ansible v2.4.1
- golang-migrate v4.19.1
- `log/slog` for structured logging

All builds and tests must use the Makefile. It exports `CGO_ENABLED=0`, and the command package contains a build guard that intentionally fails when CGO is enabled.

## Commands

```bash
make test
make vet
make build
./bin/autoadmin api
./bin/autoadmin scheduler
./bin/autoadmin worker
./bin/autoadmin migrate
```

Only the API bootstrap and infrastructure adapters are wired in the initial skeleton. Scheduler definitions, worker dispatch and migrations are intentionally enabled domain by domain after the schema baseline is generated.

## Layout

```text
cmd/autoadmin/                   process entry point and CGO guard
internal/api/                    Gin server, routing and response envelope
internal/app/                    role composition and lifecycle
internal/config/                 environment configuration
internal/modules/                Django-to-Go domain catalog
internal/platform/database/      MySQL pool and generated sqlc package
internal/messaging/rabbitmq/     durable topology, publisher and consumer
internal/scheduler/              gocron scheduler and message publication
internal/automation/ansible/     go-ansible adapter
db/schema/                       schema baseline consumed by sqlc
db/queries/                      named sqlc queries grouped by domain
db/migrations/                   post-baseline schema migrations
docs/                            architecture, contracts and migration plan
```

Use `config.example.env` as the local environment template and provide real credentials outside Git. The application does not load dotenv files itself.

See [Architecture](docs/ARCHITECTURE.md), [backend module map](docs/BACKEND_MODULE_MAP.md), [API contract](docs/API_CONTRACT.md), and [development plan](docs/DEVELOPMENT_PLAN.md).

The Django rewrite baseline is documented in:

- [Backend API analysis](docs/BACKEND_API_ANALYSIS.md)
- [Domain model analysis](docs/DOMAIN_MODEL_ANALYSIS.md)
- [Business workflows and state machines](docs/BUSINESS_WORKFLOWS.md)
- [Go rewrite implementation guide](docs/GO_REWRITE_GUIDE.md)
- [Error conventions](docs/ERROR_CONVENTIONS.md)