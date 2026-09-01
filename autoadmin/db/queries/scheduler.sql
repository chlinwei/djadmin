-- name: CountScheduledTasks :one
SELECT COUNT(*) FROM scheduler_scheduledtask
WHERE code <> 'cleanup_alert_histories'
  AND (sqlc.narg(name_pattern) IS NULL OR name LIKE sqlc.narg(name_pattern) OR code LIKE sqlc.narg(code_pattern))
  AND (sqlc.narg(enabled) IS NULL OR enabled = sqlc.narg(enabled))
  AND (sqlc.narg(is_running) IS NULL OR is_running = sqlc.narg(is_running));

-- name: ListScheduledTasks :many
SELECT t.*, m.name AS menu_name, m.path AS menu_path
FROM scheduler_scheduledtask AS t
LEFT JOIN sys_menu AS m ON m.id = t.menu_id
WHERE t.code <> 'cleanup_alert_histories'
  AND (sqlc.narg(name_pattern) IS NULL OR t.name LIKE sqlc.narg(name_pattern) OR t.code LIKE sqlc.narg(code_pattern))
  AND (sqlc.narg(enabled) IS NULL OR t.enabled = sqlc.narg(enabled))
  AND (sqlc.narg(is_running) IS NULL OR t.is_running = sqlc.narg(is_running))
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
WHERE (sqlc.narg(task_id) IS NULL OR task_id = sqlc.narg(task_id))
  AND (sqlc.narg(exact_status) IS NULL OR status = sqlc.narg(exact_status))
  AND (sqlc.narg(duration_min) IS NULL OR duration_seconds >= sqlc.narg(duration_min))
  AND (sqlc.narg(duration_max) IS NULL OR duration_seconds <= sqlc.narg(duration_max))
  AND (sqlc.narg(message_pattern) IS NULL OR message LIKE sqlc.narg(message_pattern) OR output LIKE sqlc.narg(output_pattern));

-- name: ListScheduledTaskLogs :many
SELECT l.id, t.name AS task_name, l.run_time, l.status, l.message, l.duration_seconds, l.output
FROM scheduler_scheduledtasklog AS l
JOIN scheduler_scheduledtask AS t ON t.id = l.task_id
WHERE (sqlc.narg(task_id) IS NULL OR l.task_id = sqlc.narg(task_id))
  AND (sqlc.narg(exact_status) IS NULL OR l.status = sqlc.narg(exact_status))
  AND (sqlc.narg(duration_min) IS NULL OR l.duration_seconds >= sqlc.narg(duration_min))
  AND (sqlc.narg(duration_max) IS NULL OR l.duration_seconds <= sqlc.narg(duration_max))
  AND (sqlc.narg(message_pattern) IS NULL OR l.message LIKE sqlc.narg(message_pattern) OR l.output LIKE sqlc.narg(output_pattern))
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