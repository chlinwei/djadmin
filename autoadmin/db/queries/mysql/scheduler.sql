-- name: CountScheduledTasks :one
SELECT COUNT(*) FROM scheduler_scheduledtask
WHERE code <> 'cleanup_alert_histories'
  AND (name LIKE sqlc.narg(name_pattern) OR code LIKE sqlc.narg(code_pattern) OR sqlc.narg(name_pattern) IS NULL)
  AND (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (is_running = sqlc.narg(is_running) OR sqlc.narg(is_running) IS NULL);

-- name: ListScheduledTasks :many
SELECT t.*, m.name AS menu_name, m.path AS menu_path
FROM scheduler_scheduledtask AS t
LEFT JOIN sys_menu AS m ON m.id = t.menu_id
WHERE t.code <> 'cleanup_alert_histories'
  AND (t.name LIKE sqlc.narg(name_pattern) OR t.code LIKE sqlc.narg(code_pattern) OR sqlc.narg(name_pattern) IS NULL)
  AND (t.enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (t.is_running = sqlc.narg(is_running) OR sqlc.narg(is_running) IS NULL)
ORDER BY t.id DESC
LIMIT ? OFFSET ?;

-- name: GetScheduledTask :one
SELECT t.*, m.name AS menu_name, m.path AS menu_path
FROM scheduler_scheduledtask AS t
LEFT JOIN sys_menu AS m ON m.id = t.menu_id
WHERE t.id = ? LIMIT 1;

-- name: UpdateScheduledTask :exec
UPDATE scheduler_scheduledtask
SET name = ?, code = ?, description = ?, enabled = ?, cron_expression = ?,
    interval_minutes = NULL, next_run_time = ?, update_time = ?
WHERE id = ?;

-- name: SetScheduledTaskEnabled :exec
UPDATE scheduler_scheduledtask
SET enabled = ?, next_run_time = ?, update_time = ?
WHERE id = ?;

-- name: CountScheduledTaskLogs :one
SELECT COUNT(*) FROM scheduler_scheduledtasklog
WHERE (task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (status = sqlc.narg(exact_status) OR sqlc.narg(exact_status) IS NULL)
  AND (duration_seconds >= sqlc.narg(duration_min) OR sqlc.narg(duration_min) IS NULL)
  AND (duration_seconds <= sqlc.narg(duration_max) OR sqlc.narg(duration_max) IS NULL)
  AND (message LIKE sqlc.narg(message_pattern) OR output LIKE sqlc.narg(output_pattern) OR sqlc.narg(message_pattern) IS NULL);

-- name: ListScheduledTaskLogs :many
SELECT l.id, t.name AS task_name, l.run_time, l.status, l.message, l.duration_seconds, l.output
FROM scheduler_scheduledtasklog AS l
JOIN scheduler_scheduledtask AS t ON t.id = l.task_id
WHERE (l.task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (l.status = sqlc.narg(exact_status) OR sqlc.narg(exact_status) IS NULL)
  AND (l.duration_seconds >= sqlc.narg(duration_min) OR sqlc.narg(duration_min) IS NULL)
  AND (l.duration_seconds <= sqlc.narg(duration_max) OR sqlc.narg(duration_max) IS NULL)
  AND (l.message LIKE sqlc.narg(message_pattern) OR l.output LIKE sqlc.narg(output_pattern) OR sqlc.narg(message_pattern) IS NULL)
ORDER BY l.run_time DESC
LIMIT ? OFFSET ?;

-- name: GetScheduledTaskLog :one
SELECT l.id, t.name AS task_name, l.run_time, l.status, l.message, l.duration_seconds, l.output
FROM scheduler_scheduledtasklog AS l
JOIN scheduler_scheduledtask AS t ON t.id = l.task_id
WHERE l.id = ? LIMIT 1;

-- name: ClaimScheduledTask :execresult
UPDATE scheduler_scheduledtask
SET is_running = TRUE, last_run_time = ?, update_time = ?
WHERE id = ? AND enabled = TRUE AND is_running = FALSE;

-- name: CompleteScheduledTask :exec
UPDATE scheduler_scheduledtask
SET is_running = FALSE, last_status = ?, last_message = ?, last_run_time = ?, update_time = ?
WHERE id = ?;

-- name: CreateScheduledTaskLog :exec
INSERT INTO scheduler_scheduledtasklog (
  create_time, update_time, remark, run_time, status, message,
  duration_seconds, task_id, output
) VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?);

-- name: DeleteLoginAuditsBefore :execresult
DELETE FROM audit_login_log WHERE login_time < ?;

-- name: DeleteOperationAuditsBefore :execresult
DELETE FROM audit_operation_log WHERE created_at < ?;