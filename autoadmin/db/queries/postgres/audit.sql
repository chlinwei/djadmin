-- 本文件由 make derive 从 db/queries/mysql/audit.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: CreateOperationAudit :exec
INSERT INTO audit_operation_log (
  username, user_id, method, path, route_name, client_ip, user_agent,
  status_code, duration_ms, message, created_at, request_data, response_data
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: CountOperationAudits :one
SELECT COUNT(*) FROM audit_operation_log
WHERE method <> 'GET'
  AND (username LIKE sqlc.narg(username_pattern)
    OR method LIKE sqlc.narg(method_pattern)
    OR path LIKE sqlc.narg(path_pattern)
    OR route_name LIKE sqlc.narg(route_pattern)
    OR client_ip LIKE sqlc.narg(client_ip_pattern)
    OR message LIKE sqlc.narg(message_pattern) OR sqlc.narg(username_pattern) IS NULL)
  AND (method = sqlc.narg(exact_method) OR sqlc.narg(exact_method) IS NULL)
  AND (status_code = sqlc.narg(exact_status_code) OR sqlc.narg(exact_status_code) IS NULL)
  AND (created_at >= sqlc.narg(created_from) OR sqlc.narg(created_from) IS NULL)
  AND (created_at <= sqlc.narg(created_to) OR sqlc.narg(created_to) IS NULL);

-- name: ListOperationAudits :many
SELECT * FROM audit_operation_log
WHERE method <> 'GET'
  AND (username LIKE sqlc.narg(username_pattern)
    OR method LIKE sqlc.narg(method_pattern)
    OR path LIKE sqlc.narg(path_pattern)
    OR route_name LIKE sqlc.narg(route_pattern)
    OR client_ip LIKE sqlc.narg(client_ip_pattern)
    OR message LIKE sqlc.narg(message_pattern) OR sqlc.narg(username_pattern) IS NULL)
  AND (method = sqlc.narg(exact_method) OR sqlc.narg(exact_method) IS NULL)
  AND (status_code = sqlc.narg(exact_status_code) OR sqlc.narg(exact_status_code) IS NULL)
  AND (created_at >= sqlc.narg(created_from) OR sqlc.narg(created_from) IS NULL)
  AND (created_at <= sqlc.narg(created_to) OR sqlc.narg(created_to) IS NULL)
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;