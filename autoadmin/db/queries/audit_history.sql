-- name: CountLoginAudits :one
SELECT COUNT(*) FROM audit_login_log
WHERE (sqlc.narg(keyword_pattern) IS NULL OR username LIKE sqlc.narg(keyword_pattern) OR client_ip LIKE sqlc.narg(client_ip_pattern) OR message LIKE sqlc.narg(message_pattern))
  AND (sqlc.narg(exact_status) IS NULL OR status = sqlc.narg(exact_status))
  AND (sqlc.narg(time_from) IS NULL OR login_time >= sqlc.narg(time_from))
  AND (sqlc.narg(time_to) IS NULL OR login_time <= sqlc.narg(time_to));

-- name: ListLoginAudits :many
SELECT * FROM audit_login_log
WHERE (sqlc.narg(keyword_pattern) IS NULL OR username LIKE sqlc.narg(keyword_pattern) OR client_ip LIKE sqlc.narg(client_ip_pattern) OR message LIKE sqlc.narg(message_pattern))
  AND (sqlc.narg(exact_status) IS NULL OR status = sqlc.narg(exact_status))
  AND (sqlc.narg(time_from) IS NULL OR login_time >= sqlc.narg(time_from))
  AND (sqlc.narg(time_to) IS NULL OR login_time <= sqlc.narg(time_to))
ORDER BY login_time DESC, id DESC LIMIT ? OFFSET ?;

-- name: CountWebSSHSessions :one
SELECT COUNT(*) FROM assets_webssh_session_log s JOIN assets_host h ON h.id = s.host_id
WHERE (sqlc.narg(exact_status) IS NULL OR s.status = sqlc.narg(exact_status) OR s.status = sqlc.narg(alternate_status))
  AND (sqlc.narg(exact_username) IS NULL OR s.username LIKE sqlc.narg(exact_username))
  AND (sqlc.narg(keyword_user) IS NULL OR s.username LIKE sqlc.narg(keyword_user) OR h.instance_name LIKE sqlc.narg(keyword_host) OR h.ip LIKE sqlc.narg(keyword_ip))
  AND (sqlc.narg(output_pattern) IS NULL OR s.output_content LIKE sqlc.narg(output_pattern))
  AND (sqlc.narg(time_from) IS NULL OR s.start_time >= sqlc.narg(time_from))
  AND (sqlc.narg(time_to) IS NULL OR s.start_time <= sqlc.narg(time_to));

-- name: ListWebSSHSessions :many
SELECT s.id, s.host_id AS host, COALESCE(h.instance_name, CONCAT('Host-', h.id)) AS host_name, h.ip AS host_ip,
       s.user_id, s.username, s.client_ip, s.user_agent,
      s.status,
       s.start_time, s.end_time, s.duration_seconds, s.close_code, s.error_message,
       s.input_bytes, s.command_count, s.recorded_content_bytes, s.is_content_truncated
FROM assets_webssh_session_log s JOIN assets_host h ON h.id = s.host_id
WHERE (sqlc.narg(exact_status) IS NULL OR s.status = sqlc.narg(exact_status) OR s.status = sqlc.narg(alternate_status))
  AND (sqlc.narg(exact_username) IS NULL OR s.username LIKE sqlc.narg(exact_username))
  AND (sqlc.narg(keyword_user) IS NULL OR s.username LIKE sqlc.narg(keyword_user) OR h.instance_name LIKE sqlc.narg(keyword_host) OR h.ip LIKE sqlc.narg(keyword_ip))
  AND (sqlc.narg(output_pattern) IS NULL OR s.output_content LIKE sqlc.narg(output_pattern))
  AND (sqlc.narg(time_from) IS NULL OR s.start_time >= sqlc.narg(time_from))
  AND (sqlc.narg(time_to) IS NULL OR s.start_time <= sqlc.narg(time_to))
ORDER BY s.start_time DESC, s.id DESC LIMIT ? OFFSET ?;

-- name: GetWebSSHSessionContent :one
SELECT s.id, s.status, s.start_time, s.end_time, s.duration_seconds,
       s.input_content, s.output_content, s.recorded_content_bytes, s.is_content_truncated,
       s.host_id, COALESCE(h.instance_name, CONCAT('Host-', h.id)) AS host_name, h.ip AS host_ip,
       s.username, s.effective_username, s.client_ip, s.close_code, s.error_message
FROM assets_webssh_session_log s JOIN assets_host h ON h.id = s.host_id
WHERE s.id = ? LIMIT 1;