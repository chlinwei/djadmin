-- name: CreateOperationAudit :exec
INSERT INTO audit_operation_log (
  username, user_id, method, path, route_name, client_ip, user_agent,
  status_code, duration_ms, message, created_at, request_data, response_data
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: CountOperationAudits :one
SELECT COUNT(*) FROM audit_operation_log
WHERE method <> 'GET'
  AND (sqlc.narg(username_pattern) IS NULL OR username LIKE sqlc.narg(username_pattern)
    OR method LIKE sqlc.narg(method_pattern)
    OR path LIKE sqlc.narg(path_pattern)
    OR route_name LIKE sqlc.narg(route_pattern)
    OR client_ip LIKE sqlc.narg(client_ip_pattern)
    OR message LIKE sqlc.narg(message_pattern))
  AND (sqlc.narg(exact_method) IS NULL OR method = sqlc.narg(exact_method))
  AND (sqlc.narg(exact_status_code) IS NULL OR status_code = sqlc.narg(exact_status_code))
  AND (sqlc.narg(created_from) IS NULL OR created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_to) IS NULL OR created_at <= sqlc.narg(created_to));

-- name: ListOperationAudits :many
SELECT * FROM audit_operation_log
WHERE method <> 'GET'
  AND (sqlc.narg(username_pattern) IS NULL OR username LIKE sqlc.narg(username_pattern)
    OR method LIKE sqlc.narg(method_pattern)
    OR path LIKE sqlc.narg(path_pattern)
    OR route_name LIKE sqlc.narg(route_pattern)
    OR client_ip LIKE sqlc.narg(client_ip_pattern)
    OR message LIKE sqlc.narg(message_pattern))
  AND (sqlc.narg(exact_method) IS NULL OR method = sqlc.narg(exact_method))
  AND (sqlc.narg(exact_status_code) IS NULL OR status_code = sqlc.narg(exact_status_code))
  AND (sqlc.narg(created_from) IS NULL OR created_at >= sqlc.narg(created_from))
  AND (sqlc.narg(created_to) IS NULL OR created_at <= sqlc.narg(created_to))
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?;