-- name: ListInspectionTargetExecutions :many
SELECT id, deployment_id AS deployment, host_id AS host, target_name, host_id_snapshot,
       host_ip_snapshot, agent_id_snapshot, status, passed, error_message, raw_result,
       start_time, end_time
FROM inspection_target_execution
WHERE execution_id = ?
ORDER BY id;

-- name: GetInspectionGroup :one
SELECT id, name, scope, description, enabled, create_time, update_time
FROM inspection_group WHERE id = sqlc.arg(id);

-- name: ListInspectionChecksByGroup :many
SELECT id, name, executor, execution_location, config, severity, enabled, `order`
FROM inspection_check WHERE group_id = sqlc.arg(group_id) ORDER BY `order`, id;

-- name: CountInspectionGroups :one
SELECT COUNT(*) FROM inspection_group
WHERE (sqlc.narg(pattern) IS NULL OR name LIKE sqlc.narg(pattern) OR description LIKE sqlc.narg(pattern));

-- name: ListInspectionGroups :many
SELECT id, name, scope, description, enabled, create_time, update_time
FROM inspection_group
WHERE (sqlc.narg(pattern) IS NULL OR name LIKE sqlc.narg(pattern) OR description LIKE sqlc.narg(pattern))
ORDER BY name, id
LIMIT ? OFFSET ?;

-- name: CountInspectionTasks :one
SELECT COUNT(*) FROM inspection_task t
JOIN inspection_group g ON g.id = t.group_id
LEFT JOIN assets_application_service s ON s.id = t.logical_service_id
WHERE (sqlc.narg(pattern) IS NULL OR t.name LIKE sqlc.narg(pattern) OR g.name LIKE sqlc.narg(pattern) OR s.name LIKE sqlc.narg(pattern));

-- name: ListInspectionTasksTyped :many
SELECT t.id, t.name, t.inspection_name, t.group_id AS `group`, g.name AS group_name, g.scope AS scope,
       t.logical_service_id AS logical_service, COALESCE(s.name,'') AS logical_service_name,
       t.selected_host_ids, t.concurrency, t.timeout_seconds, t.cron_expression, t.next_run_time,
       t.last_run_time, t.enabled, t.create_time, t.update_time
FROM inspection_task t
JOIN inspection_group g ON g.id = t.group_id
LEFT JOIN assets_application_service s ON s.id = t.logical_service_id
WHERE (sqlc.narg(pattern) IS NULL OR t.name LIKE sqlc.narg(pattern) OR g.name LIKE sqlc.narg(pattern) OR s.name LIKE sqlc.narg(pattern))
ORDER BY t.id DESC
LIMIT ? OFFSET ?;

-- name: GetInspectionTask :one
SELECT t.id, t.name, t.inspection_name, t.group_id AS `group`, g.name AS group_name, g.scope AS scope,
       t.logical_service_id AS logical_service, COALESCE(s.name,'') AS logical_service_name,
       t.selected_host_ids, t.concurrency, t.timeout_seconds, t.cron_expression, t.next_run_time,
       t.last_run_time, t.enabled, t.create_time, t.update_time
FROM inspection_task t
JOIN inspection_group g ON g.id = t.group_id
LEFT JOIN assets_application_service s ON s.id = t.logical_service_id
WHERE t.id = sqlc.arg(id);

-- name: ListEnabledInspectionChecksForRun :many
SELECT name, executor, execution_location, config, severity, `order`
FROM inspection_check
WHERE group_id = sqlc.arg(group_id) AND enabled = TRUE
ORDER BY `order`, id;

-- name: ListHostGroupTreeNodes :many
SELECT id, name, parent_id
FROM assets_hostgroup
ORDER BY name, id;

-- name: ListHostScopeTreeHosts :many
SELECT id, instance_name, ip, group_id, agent_id
FROM assets_host
WHERE is_deleted_in_cloud = FALSE
ORDER BY instance_name, id;

-- name: CountInspectionExecutions :one
SELECT COUNT(*) FROM inspection_execution e
WHERE (sqlc.narg(task_id) IS NULL OR e.task_id = sqlc.narg(task_id))
  AND (sqlc.narg(status) IS NULL OR e.status = sqlc.narg(status))
  AND (sqlc.narg(trigger_type) IS NULL OR e.trigger_type = sqlc.narg(trigger_type))
  AND (sqlc.narg(start_time) IS NULL OR e.create_time >= sqlc.narg(start_time))
  AND (sqlc.narg(end_time) IS NULL OR e.create_time <= sqlc.narg(end_time));

-- name: ListInspectionExecutions :many
SELECT e.id, e.task_id AS task, COALESCE(t.name,'') AS task_name,
       COALESCE(JSON_UNQUOTE(JSON_EXTRACT(e.service_snapshot,'$.name')),'') AS target_name,
       e.status, e.trigger_type, e.summary, e.requested_username, e.start_time, e.end_time, e.create_time
FROM inspection_execution e
LEFT JOIN inspection_task t ON t.id = e.task_id
WHERE (sqlc.narg(task_id) IS NULL OR e.task_id = sqlc.narg(task_id))
  AND (sqlc.narg(status) IS NULL OR e.status = sqlc.narg(status))
  AND (sqlc.narg(trigger_type) IS NULL OR e.trigger_type = sqlc.narg(trigger_type))
  AND (sqlc.narg(start_time) IS NULL OR e.create_time >= sqlc.narg(start_time))
  AND (sqlc.narg(end_time) IS NULL OR e.create_time <= sqlc.narg(end_time))
ORDER BY e.id DESC
LIMIT ? OFFSET ?;

-- name: GetInspectionExecutionTyped :one
SELECT e.id, e.task_id AS task, COALESCE(t.name,'') AS task_name, e.status, e.trigger_type,
       e.task_snapshot, e.group_snapshot, e.service_snapshot, e.target_snapshot, e.summary,
       e.requested_username, e.start_time, e.end_time, e.create_time
FROM inspection_execution e
LEFT JOIN inspection_task t ON t.id = e.task_id
WHERE e.id = sqlc.arg(id);

-- name: ListInspectionResultsByTarget :many
SELECT id, check_key, check_type, name, status, severity, expected_value, actual_value, message
FROM inspection_result WHERE target_id = sqlc.arg(target_id) ORDER BY id;