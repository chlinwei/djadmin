-- 本文件由 make derive 从 db/queries/mysql/assets_webssh.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: CreateWebSSHSession :one
INSERT INTO assets_webssh_session_log (
  host_id, user_id, username, client_ip, user_agent, status, start_time,
  error_message, input_bytes, command_count, input_content, output_content,
  recorded_content_bytes, is_content_truncated, effective_username,
  requested_username, switch_user_error, switch_user_status
) VALUES (
  sqlc.arg(host_id), sqlc.narg(user_id), sqlc.arg(username), sqlc.arg(client_ip),
  sqlc.arg(user_agent), 'connected', sqlc.arg(start_time),
  '', 0, 0, '', '', 0, false, '', sqlc.arg(requested_username), '', 'pending'
)
RETURNING id;

-- name: GetWebSSHAuditID :one
SELECT id FROM assets_webssh_session_log WHERE id = $1;

-- name: UpdateWebSSHSession :execrows
UPDATE assets_webssh_session_log
SET status = sqlc.arg(status), end_time = sqlc.narg(end_time),
    duration_seconds = sqlc.narg(duration_seconds), close_code = sqlc.narg(close_code),
    error_message = sqlc.arg(error_message), input_bytes = sqlc.arg(input_bytes),
    command_count = sqlc.arg(command_count), input_content = sqlc.arg(input_content),
    output_content = sqlc.arg(output_content), recorded_content_bytes = sqlc.arg(recorded_content_bytes),
    is_content_truncated = sqlc.arg(is_content_truncated), effective_username = sqlc.arg(effective_username),
    switch_user_error = sqlc.arg(switch_user_error), switch_user_status = sqlc.arg(switch_user_status)
WHERE id = sqlc.arg(id);
