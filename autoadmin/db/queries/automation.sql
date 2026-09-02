-- name: CountInventories :one
SELECT COUNT(*) FROM automation_inventory a
WHERE (sqlc.narg(pattern) IS NULL OR a.name LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern));

-- name: ListInventoriesTyped :many
SELECT * FROM automation_inventory a
WHERE (sqlc.narg(pattern) IS NULL OR a.name LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern))
ORDER BY a.id DESC
LIMIT ? OFFSET ?;

-- name: GetInventoryTyped :one
SELECT * FROM automation_inventory WHERE id = sqlc.arg(id);

-- name: CountTasks :one
SELECT COUNT(*) FROM automation_task a
LEFT JOIN automation_playbook_template p ON p.id = a.playbook_template_id
LEFT JOIN automation_inventory i ON i.id = a.inventory_id
WHERE (sqlc.narg(id) IS NULL OR a.id = sqlc.narg(id))
  AND (sqlc.narg(pattern) IS NULL OR a.name LIKE sqlc.narg(pattern) OR p.name LIKE sqlc.narg(pattern) OR i.name LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern));

-- name: ListTasksTyped :many
SELECT a.id, a.create_time, a.update_time, a.remark, a.name, a.env_vars, a.enabled, a.inventory_id,
       a.default_limit, a.execution_timeout_seconds, a.playbook_template_id, a.run_as_user, a.run_as_group, a.work_directory,
       COALESCE(p.name,'') AS raw_template_name, COALESCE(i.name,'') AS inventory_name
FROM automation_task a
LEFT JOIN automation_playbook_template p ON p.id = a.playbook_template_id
LEFT JOIN automation_inventory i ON i.id = a.inventory_id
WHERE (sqlc.narg(id) IS NULL OR a.id = sqlc.narg(id))
  AND (sqlc.narg(pattern) IS NULL OR a.name LIKE sqlc.narg(pattern) OR p.name LIKE sqlc.narg(pattern) OR i.name LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern))
ORDER BY a.id DESC
LIMIT ? OFFSET ?;

-- name: GetTaskTyped :one
SELECT t.id, t.create_time, t.update_time, t.remark, t.name, t.env_vars, t.enabled, t.inventory_id,
       t.default_limit, t.execution_timeout_seconds, t.playbook_template_id, t.run_as_user, t.run_as_group, t.work_directory,
       p.name AS template_name, p.content AS template_content, COALESCE(i.name,'') AS inventory_name
FROM automation_task t
JOIN automation_playbook_template p ON p.id = t.playbook_template_id
LEFT JOIN automation_inventory i ON i.id = t.inventory_id
WHERE t.id = sqlc.arg(id);

-- name: CountJobs :one
SELECT COUNT(*) FROM automation_execution_job a
WHERE (sqlc.narg(id) IS NULL OR a.id = sqlc.narg(id))
  AND (sqlc.narg(status) IS NULL OR a.status = sqlc.narg(status))
  AND (sqlc.narg(task_id) IS NULL OR a.task_id = sqlc.narg(task_id))
  AND (sqlc.narg(pattern) IS NULL OR a.requested_username LIKE sqlc.narg(pattern) OR a.template_name_snapshot LIKE sqlc.narg(pattern) OR a.task_name_snapshot LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern));

-- name: ListJobsTyped :many
SELECT * FROM automation_execution_job a
WHERE (sqlc.narg(id) IS NULL OR a.id = sqlc.narg(id))
  AND (sqlc.narg(status) IS NULL OR a.status = sqlc.narg(status))
  AND (sqlc.narg(task_id) IS NULL OR a.task_id = sqlc.narg(task_id))
  AND (sqlc.narg(pattern) IS NULL OR a.requested_username LIKE sqlc.narg(pattern) OR a.template_name_snapshot LIKE sqlc.narg(pattern) OR a.task_name_snapshot LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern))
ORDER BY a.id DESC
LIMIT ? OFFSET ?;

-- name: GetJobTyped :one
SELECT j.*, COALESCE(t.execution_timeout_seconds,600) AS execution_timeout_seconds
FROM automation_execution_job j
LEFT JOIN automation_task t ON t.id = j.task_id
WHERE j.id = sqlc.arg(id);
