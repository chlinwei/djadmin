-- 本文件由 make derive 从 db/queries/mysql/maintenance_window_queries.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: GetMaintenanceHost :one
SELECT id, COALESCE(instance_name,'') AS name FROM assets_host WHERE id=sqlc.arg(id);

-- name: CountMaintenanceWindows :one
SELECT COUNT(*) FROM monitor_maintenance_window WHERE status <> 'deleted';

-- name: ListMaintenanceWindows :many
SELECT * FROM monitor_maintenance_window WHERE status <> 'deleted' ORDER BY starts_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: GetMaintenanceWindow :one
SELECT * FROM monitor_maintenance_window WHERE id=sqlc.arg(id) AND status <> 'deleted';

-- name: LockMaintenanceWindow :one
SELECT * FROM monitor_maintenance_window WHERE id=sqlc.arg(id) AND status <> 'deleted' FOR UPDATE;

-- name: ListActiveMaintenanceWindows :many
SELECT * FROM monitor_maintenance_window WHERE status='open'
 AND starts_at <= sqlc.arg(start_bound) AND ends_at > sqlc.arg(end_bound)
ORDER BY id;

-- name: CreateMaintenanceWindow :one
INSERT INTO monitor_maintenance_window
(title,reason,starts_at,ends_at,matchers,status,created_by,updated_by,created_at,updated_at)
VALUES (sqlc.arg(title),sqlc.arg(reason),sqlc.arg(starts_at),sqlc.arg(ends_at),sqlc.arg(matchers),'open',
 sqlc.arg(created_by),sqlc.arg(updated_by),sqlc.arg(created_at),sqlc.arg(updated_at))
RETURNING id;

-- name: EditMaintenanceWindow :exec
UPDATE monitor_maintenance_window SET title=sqlc.arg(title),reason=sqlc.arg(reason),
starts_at=sqlc.arg(starts_at),ends_at=sqlc.arg(ends_at),matchers=sqlc.arg(matchers),
updated_by=sqlc.arg(updated_by),updated_at=sqlc.arg(updated_at) WHERE id=sqlc.arg(id);

-- name: CloseMaintenanceWindow :exec
UPDATE monitor_maintenance_window SET status=sqlc.arg(status),
ends_at=sqlc.arg(ends_at),updated_by=sqlc.arg(updated_by),updated_at=sqlc.arg(updated_at) WHERE id=sqlc.arg(id);

-- name: AddMaintenanceWindowAudit :exec
INSERT INTO monitor_maintenance_window_audit (window_id,action,actor,decided_at,snapshot)
VALUES (sqlc.arg(window_id),sqlc.arg(action),sqlc.arg(actor),sqlc.arg(decided_at),sqlc.arg(snapshot));
