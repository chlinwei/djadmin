# Development plan

## Phase 0: executable foundation

- [x] Create the Go 1.27 module beside `backend`.
- [x] Enforce `CGO_ENABLED=0` through Makefile and a compile-time guard.
- [x] Add Gin, MySQL, RabbitMQ, gocron and go-ansible adapters.
- [x] Preserve the response envelope and public route boundaries.
- [ ] Add `slog` JSON logging, request ids, metrics and configuration tests.

Exit criterion: `make all` succeeds and API liveness/readiness checks pass against a development MySQL instance.

## Phase 1: schema baseline and contract harness

- [x] Verify the configured development MySQL and applied Django migration graph.
- [x] Capture live DDL for the first identity/RBAC/config tables in `db/schema`.
- [x] Add typed sqlc queries and repositories for user, role, menu and system configuration.
- [ ] Export the remaining fully migrated Django MySQL schema into `db/schema`.
- [ ] Baseline golang-migrate without recreating existing tables.
- [ ] Capture representative Django API fixtures and build differential contract tests.

Exit criterion: generated types match nullable fields, unsigned values, decimals, JSON and timestamps in the live schema.

## Phase 2: identity and RBAC

- [x] Implement Django PBKDF2 verification for the hash format present in the migrated database.
- [x] Implement HS256 JWT compatibility, login, current user and login audit persistence.
- [x] Add fail-closed permission middleware with the existing admin bypass.
- [ ] Implement the remaining user center and API token APIs.
- Implement users, roles, menus and assignment transactions.
- Complete the route-to-`module:resource:action` permission matrix.
- [x] Write login and operation audit records, with sensitive-field redaction and filtered operation-log queries.

Exit criterion: the existing frontend can authenticate against Go and pass user/role/menu end-to-end tests unchanged.

## Phase 3: assets and agent gateway

- [x] Migrate projects, business systems, environments, credentials, host groups and base host CRUD/list APIs.
- [x] Migrate application, application-version and cluster-profile catalog CRUD/list APIs.
- [x] Add credential masking and versioned reversible encryption for credential writes.
- [ ] Complete host tree, recursive filters/deletion, batch operations and collected-detail projections.
- [ ] Migrate deployment templates, application services and application deployments.
- Extract agent gRPC session ownership into an explicit gateway.
- Migrate collection, WebSSH and file operations only after gateway reconnect and multi-instance behavior is tested.

Exit criterion: assets pages and agent lifecycle work without process-local session assumptions.

## Phase 4: scheduler and workers

- [x] Implement scheduler task list/detail/edit/enable/disable/status and execution-log APIs.
- [x] Validate five-field cron expressions and calculate `next_run_time` through gocron v2.
- [x] Implement the persisted global scheduler desired-state switch.
- [x] Connect API run-now, gocron Scheduler and Worker through RabbitMQ with idempotent task claims.
- [x] Migrate login-audit and operation-audit cleanup handlers; register only task codes supported by the Go worker.
- [ ] Reload enabled definitions dynamically after task edits instead of requiring Scheduler restart.
- Add a MySQL leader lease for scheduler high availability.
- Persist execution rows before publication and implement an outbox publisher.
- Add bounded retry queues, dead-letter inspection and idempotent worker claims.
- Replace Celery maintenance jobs after parity tests.

Exit criterion: restart, duplicate-delivery, RabbitMQ outage and scheduler failover tests do not lose or duplicate business execution.

## Phase 5: automation and inspection

- Migrate inventory, task, playbook, job and workflow APIs.
- Execute Ansible only in workers and persist per-target output incrementally.
- Preserve immutable snapshots and cancellation semantics.
- Migrate inspection scopes, concurrency, warning severity and offline-agent behavior.

Exit criterion: prechecks, execution histories, cancellation and target summaries match Django.

## Phase 6: monitoring, logs and audit

- Migrate exporter targets, packages and installation history.
- Migrate Alertmanager webhook ingestion, routing, deliveries and recipient bindings.
- Migrate OpenSearch and log collection configuration.
- Migrate retention jobs and complete audit parity.

Exit criterion: machine callbacks, notifications and log-management workflows pass integration tests.

## Phase 7: traffic migration and retirement

- Route one completed domain at a time to Go; do not add permanent compatibility branches.
- Compare error rates, latency, database writes and audit records.
- Keep a rollback window at the routing layer.
- Remove the corresponding Django path after the observation window and update ownership docs.

Exit criterion: all backend traffic and scheduled work run on Go, with Django and Celery removed from deployment.

## Required checks

Use only Makefile targets for Go compilation:

```bash
make fmt
make vet
make test
make build
```

Integration suites must provision MySQL and RabbitMQ, verify UTC behavior, run with the race detector where supported, and confirm the built binary is statically linked.