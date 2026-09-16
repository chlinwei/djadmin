-- 本文件由 make derive 从 db/queries/mysql/audit_history.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: CountLoginAudits :one
SELECT COUNT(*) FROM audit_login_log
WHERE (username LIKE sqlc.narg(keyword_pattern) OR client_ip LIKE sqlc.narg(client_ip_pattern) OR message LIKE sqlc.narg(message_pattern) OR sqlc.narg(keyword_pattern) IS NULL)
  AND (status = sqlc.narg(exact_status) OR sqlc.narg(exact_status) IS NULL)
  AND (login_time >= sqlc.narg(time_from) OR sqlc.narg(time_from) IS NULL)
  AND (login_time <= sqlc.narg(time_to) OR sqlc.narg(time_to) IS NULL);

-- name: ListLoginAudits :many
SELECT * FROM audit_login_log
WHERE (username LIKE sqlc.narg(keyword_pattern) OR client_ip LIKE sqlc.narg(client_ip_pattern) OR message LIKE sqlc.narg(message_pattern) OR sqlc.narg(keyword_pattern) IS NULL)
  AND (status = sqlc.narg(exact_status) OR sqlc.narg(exact_status) IS NULL)
  AND (login_time >= sqlc.narg(time_from) OR sqlc.narg(time_from) IS NULL)
  AND (login_time <= sqlc.narg(time_to) OR sqlc.narg(time_to) IS NULL)
ORDER BY login_time DESC, id DESC LIMIT $1 OFFSET $2;

-- name: CountWebSSHSessions :one
SELECT COUNT(*) FROM assets_webssh_session_log s JOIN assets_host h ON h.id = s.host_id
WHERE (s.status = sqlc.narg(exact_status) OR s.status = sqlc.narg(alternate_status) OR sqlc.narg(exact_status) IS NULL)
  AND (s.username LIKE sqlc.narg(exact_username) OR sqlc.narg(exact_username) IS NULL)
  AND (s.username LIKE sqlc.narg(keyword_user) OR h.instance_name LIKE sqlc.narg(keyword_host) OR h.ip LIKE sqlc.narg(keyword_ip) OR sqlc.narg(keyword_user) IS NULL)
  AND (s.output_content LIKE sqlc.narg(output_pattern) OR sqlc.narg(output_pattern) IS NULL)
  AND (s.start_time >= sqlc.narg(time_from) OR sqlc.narg(time_from) IS NULL)
  AND (s.start_time <= sqlc.narg(time_to) OR sqlc.narg(time_to) IS NULL);

-- name: ListWebSSHSessions :many
SELECT s.id, s.host_id AS host, COALESCE(h.instance_name, CONCAT('Host-', h.id)) AS host_name, h.ip AS host_ip,
       s.user_id, s.username, s.client_ip, s.user_agent,
      s.status,
       s.start_time, s.end_time, s.duration_seconds, s.close_code, s.error_message,
       s.input_bytes, s.command_count, s.recorded_content_bytes, s.is_content_truncated
FROM assets_webssh_session_log s JOIN assets_host h ON h.id = s.host_id
WHERE (s.status = sqlc.narg(exact_status) OR s.status = sqlc.narg(alternate_status) OR sqlc.narg(exact_status) IS NULL)
  AND (s.username LIKE sqlc.narg(exact_username) OR sqlc.narg(exact_username) IS NULL)
  AND (s.username LIKE sqlc.narg(keyword_user) OR h.instance_name LIKE sqlc.narg(keyword_host) OR h.ip LIKE sqlc.narg(keyword_ip) OR sqlc.narg(keyword_user) IS NULL)
  AND (s.output_content LIKE sqlc.narg(output_pattern) OR sqlc.narg(output_pattern) IS NULL)
  AND (s.start_time >= sqlc.narg(time_from) OR sqlc.narg(time_from) IS NULL)
  AND (s.start_time <= sqlc.narg(time_to) OR sqlc.narg(time_to) IS NULL)
ORDER BY s.start_time DESC, s.id DESC LIMIT $1 OFFSET $2;

-- name: GetWebSSHSessionContent :one
SELECT s.id, s.status, s.start_time, s.end_time, s.duration_seconds,
       s.input_content, s.output_content, s.recorded_content_bytes, s.is_content_truncated,
       s.host_id, COALESCE(h.instance_name, CONCAT('Host-', h.id)) AS host_name, h.ip AS host_ip,
       s.username, s.effective_username, s.client_ip, s.close_code, s.error_message
FROM assets_webssh_session_log s JOIN assets_host h ON h.id = s.host_id
WHERE s.id = $1 LIMIT 1;