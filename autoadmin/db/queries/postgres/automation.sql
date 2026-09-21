-- 本文件由 make derive 从 db/queries/mysql/automation.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: CountInventories :one
SELECT COUNT(*) FROM automation_inventory a
WHERE (a.name LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListInventoriesTyped :many
SELECT * FROM automation_inventory a
WHERE (a.name LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY a.id DESC
LIMIT $1 OFFSET $2;

-- name: GetInventoryTyped :one
SELECT * FROM automation_inventory WHERE id = sqlc.arg(id);

-- name: CountTasks :one
SELECT COUNT(*) FROM automation_task a
LEFT JOIN automation_playbook_template p ON p.id = a.playbook_template_id
LEFT JOIN automation_inventory i ON i.id = a.inventory_id
WHERE (a.id = sqlc.narg(id) OR sqlc.narg(id) IS NULL)
  AND (a.name LIKE sqlc.narg(pattern) OR p.name LIKE sqlc.narg(pattern) OR i.name LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListTasksTyped :many
SELECT a.id, a.create_time, a.update_time, a.remark, a.name, a.env_vars, a.enabled, a.inventory_id,
       a.default_limit, a.execution_timeout_seconds, a.playbook_template_id, a.run_as_user, a.run_as_group, a.work_directory,
       COALESCE(p.name,'') AS raw_template_name, COALESCE(i.name,'') AS inventory_name
FROM automation_task a
LEFT JOIN automation_playbook_template p ON p.id = a.playbook_template_id
LEFT JOIN automation_inventory i ON i.id = a.inventory_id
WHERE (a.id = sqlc.narg(id) OR sqlc.narg(id) IS NULL)
  AND (a.name LIKE sqlc.narg(pattern) OR p.name LIKE sqlc.narg(pattern) OR i.name LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY a.id DESC
LIMIT $1 OFFSET $2;

-- name: GetTaskTyped :one
SELECT t.id, t.create_time, t.update_time, t.remark, t.name, t.env_vars, t.enabled, t.inventory_id,
       t.default_limit, t.execution_timeout_seconds, t.playbook_template_id, t.run_as_user, t.run_as_group, t.work_directory,
       p.name AS template_name, p.content AS template_content, p.content_format AS template_content_format, COALESCE(i.name,'') AS inventory_name
FROM automation_task t
JOIN automation_playbook_template p ON p.id = t.playbook_template_id
LEFT JOIN automation_inventory i ON i.id = t.inventory_id
WHERE t.id = sqlc.arg(id);

-- name: CountJobs :one
SELECT COUNT(*) FROM automation_execution_job a
WHERE (a.id = sqlc.narg(id) OR sqlc.narg(id) IS NULL)
  AND (a.status = sqlc.narg(status) OR sqlc.narg(status) IS NULL)
  AND (a.task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (a.source = sqlc.narg(source) OR sqlc.narg(source) IS NULL)
  AND (a.requested_username LIKE sqlc.narg(pattern) OR a.template_name_snapshot LIKE sqlc.narg(pattern) OR a.task_name_snapshot LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListJobsTyped :many
SELECT * FROM automation_execution_job a
WHERE (a.id = sqlc.narg(id) OR sqlc.narg(id) IS NULL)
  AND (a.status = sqlc.narg(status) OR sqlc.narg(status) IS NULL)
  AND (a.task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (a.source = sqlc.narg(source) OR sqlc.narg(source) IS NULL)
  AND (a.requested_username LIKE sqlc.narg(pattern) OR a.template_name_snapshot LIKE sqlc.narg(pattern) OR a.task_name_snapshot LIKE sqlc.narg(pattern) OR a.remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY a.id DESC
LIMIT $1 OFFSET $2;

-- name: GetJobTyped :one
SELECT j.*, COALESCE(t.execution_timeout_seconds,600) AS execution_timeout_seconds
FROM automation_execution_job j
LEFT JOIN automation_task t ON t.id = j.task_id
WHERE j.id = sqlc.arg(id);

-- ---- P2-2：automation 包内联 SQL 的收纳处（模板 / Inventory / 任务 / 作业运行期 / 主机解析）----
--
-- 与 inspection 同一套约定：时间由应用层传（NOW()/UTC_TIMESTAMP 是方言函数）、PATCH 合并在
-- 应用层做（不用 COALESCE(narg(...), col)，可空布尔/JSON 在两侧的推导不同）、可变长 IN 用
-- `= ANY(sqlc.arg(x)::bigint[])`（派生脚本会改写成 PG 的 `= ANY(sqlc.arg(x)::bigint[])`，见 P4-7）。

-- 模板列表：原实现运行时拼 `WHERE (?='' OR name LIKE ? ...)`，改成 sqlc.narg 可选过滤。
-- name: CountAutomationPlaybooks :one
SELECT COUNT(*) FROM automation_playbook_template
WHERE (name LIKE sqlc.narg(pattern) OR description LIKE sqlc.narg(pattern)
       OR COALESCE(remark, '') LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
  AND (category = sqlc.narg(category) OR sqlc.narg(category) IS NULL);

-- 排序：原实现按白名单拼列名与方向（`ORDER BY <column> <dir>`）。标识符不能被参数化，
-- 改成按 sort_key（带 '-' 前缀表示倒序）选择的 CASE 表达式——两方言等价，代价是排序
-- 不再走索引（模板表很小，可接受）；未命中任何分支时回落到 `id DESC`。
-- name: ListAutomationPlaybooks :many
SELECT id, create_time, update_time, remark, name, description, content, content_format, category
FROM automation_playbook_template
WHERE (name LIKE sqlc.narg(pattern) OR description LIKE sqlc.narg(pattern)
       OR COALESCE(remark, '') LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
  AND (category = sqlc.narg(category) OR sqlc.narg(category) IS NULL)
ORDER BY
  CASE WHEN sqlc.narg(sort_key) = 'id' THEN id END ASC,
  CASE WHEN sqlc.narg(sort_key) = '-id' THEN id END DESC,
  CASE WHEN sqlc.narg(sort_key) = 'name' THEN name END ASC,
  CASE WHEN sqlc.narg(sort_key) = '-name' THEN name END DESC,
  CASE WHEN sqlc.narg(sort_key) = 'create_time' THEN create_time END ASC,
  CASE WHEN sqlc.narg(sort_key) = '-create_time' THEN create_time END DESC,
  CASE WHEN sqlc.narg(sort_key) = 'update_time' THEN update_time END ASC,
  CASE WHEN sqlc.narg(sort_key) = '-update_time' THEN update_time END DESC,
  id DESC
LIMIT $1 OFFSET $2;

-- name: GetAutomationPlaybook :one
SELECT id, create_time, update_time, remark, name, description, content, content_format, category
FROM automation_playbook_template WHERE id = sqlc.arg(id);

-- name: CreateAutomationPlaybook :one
INSERT INTO automation_playbook_template(create_time, update_time, remark, name, description, content, content_format, category)
VALUES (sqlc.arg(create_time), sqlc.arg(update_time), sqlc.narg(remark), sqlc.arg(name),
        sqlc.arg(description), sqlc.arg(content), sqlc.arg(content_format), sqlc.arg(category))
RETURNING id;

-- name: UpdateAutomationPlaybook :exec
UPDATE automation_playbook_template
SET update_time = sqlc.arg(update_time), remark = sqlc.narg(remark), name = sqlc.arg(name),
    description = sqlc.arg(description), content = sqlc.arg(content), content_format = sqlc.arg(content_format),
    category = sqlc.arg(category)
WHERE id = sqlc.arg(id);

-- name: UpdateAutomationPlaybookContent :execrows
UPDATE automation_playbook_template
SET content = sqlc.arg(content), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: DeleteAutomationPlaybook :execrows
DELETE FROM automation_playbook_template WHERE id = sqlc.arg(id);

-- Inventory 的主机选项（新建 Inventory 时的选择器）：同样是运行时拼 WHERE 改成 narg。
-- name: CountAutomationHostOptions :one
SELECT COUNT(*) FROM assets_host h
LEFT JOIN assets_hostsystem s ON s.host_id = h.id
WHERE h.ip IS NOT NULL
  AND (h.instance_name LIKE sqlc.narg(pattern) OR s.hostname LIKE sqlc.narg(pattern)
       OR h.ip LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListAutomationHostOptions :many
SELECT h.id, h.instance_name, s.hostname, h.ip, h.group_id
FROM assets_host h
LEFT JOIN assets_hostsystem s ON s.host_id = h.id
WHERE h.ip IS NOT NULL
  AND (h.instance_name LIKE sqlc.narg(pattern) OR s.hostname LIKE sqlc.narg(pattern)
       OR h.ip LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY h.id
LIMIT $1 OFFSET $2;

-- name: ListAutomationHostGroupTree :many
SELECT id, name, parent_id FROM assets_hostgroup ORDER BY id;

-- name: CreateAutomationInventory :one
INSERT INTO automation_inventory(create_time, update_time, remark, name, selected_host_ids, enabled,
                                 update_on_launch, update_cache_timeout, last_sync_status, last_sync_message,
                                 last_sync_host_count)
VALUES (sqlc.arg(create_time), sqlc.arg(update_time), sqlc.narg(remark), sqlc.arg(name),
        sqlc.arg(selected_host_ids), sqlc.arg(enabled), sqlc.arg(update_on_launch),
        sqlc.arg(update_cache_timeout), 'never', '', 0)
RETURNING id;

-- name: UpdateAutomationInventory :exec
UPDATE automation_inventory
SET update_time = sqlc.arg(update_time), remark = sqlc.narg(remark), name = sqlc.arg(name),
    selected_host_ids = sqlc.arg(selected_host_ids), enabled = sqlc.arg(enabled),
    update_on_launch = sqlc.arg(update_on_launch), update_cache_timeout = sqlc.arg(update_cache_timeout)
WHERE id = sqlc.arg(id);

-- name: DeleteAutomationInventory :execrows
DELETE FROM automation_inventory WHERE id = sqlc.arg(id);

-- name: CreateAutomationTask :one
INSERT INTO automation_task(create_time, update_time, remark, name, playbook_template_id, inventory_id,
                            env_vars, default_limit, enabled, execution_timeout_seconds, run_as_user,
                            run_as_group, work_directory)
VALUES (sqlc.arg(create_time), sqlc.arg(update_time), sqlc.narg(remark), sqlc.arg(name),
        sqlc.arg(playbook_template_id), sqlc.narg(inventory_id), sqlc.arg(env_vars), sqlc.arg(default_limit),
        sqlc.arg(enabled), sqlc.arg(execution_timeout_seconds), sqlc.arg(run_as_user), sqlc.arg(run_as_group),
        sqlc.arg(work_directory))
RETURNING id;

-- name: UpdateAutomationTask :exec
UPDATE automation_task
SET update_time = sqlc.arg(update_time), remark = sqlc.narg(remark), name = sqlc.arg(name),
    playbook_template_id = sqlc.arg(playbook_template_id), inventory_id = sqlc.narg(inventory_id),
    env_vars = sqlc.arg(env_vars), default_limit = sqlc.arg(default_limit), enabled = sqlc.arg(enabled),
    execution_timeout_seconds = sqlc.arg(execution_timeout_seconds), run_as_user = sqlc.arg(run_as_user),
    run_as_group = sqlc.arg(run_as_group), work_directory = sqlc.arg(work_directory)
WHERE id = sqlc.arg(id);

-- 列表里的启用开关只提交 enabled，其余字段保持原值（走这一条，不当成整表单提交）。
-- name: SetAutomationTaskEnabled :exec
UPDATE automation_task SET enabled = sqlc.arg(enabled), update_time = sqlc.arg(update_time) WHERE id = sqlc.arg(id);

-- name: DeleteAutomationTask :execrows
DELETE FROM automation_task WHERE id = sqlc.arg(id);

-- name: CreateAutomationJob :one
INSERT INTO automation_execution_job(create_time, update_time, remark, job_id, task_id, status, trigger_type, source,
                                     inventory_snapshot, task_name_snapshot, template_name_snapshot,
                                     template_content_snapshot, template_content_format_snapshot, extra_vars, "limit", result_summary,
                                     run_as_user_snapshot, run_as_group_snapshot, work_directory_snapshot,
                                     requested_user_id, requested_username)
VALUES (sqlc.arg(create_time), sqlc.arg(update_time), NULL, sqlc.arg(job_id), sqlc.narg(task_id), 'pending', 'manual', 'manual',
        sqlc.arg(inventory_snapshot), sqlc.arg(task_name_snapshot), sqlc.arg(template_name_snapshot),
        sqlc.arg(template_content_snapshot), sqlc.arg(template_content_format_snapshot), sqlc.arg(extra_vars), sqlc.arg(job_limit), sqlc.arg(result_summary),
        sqlc.arg(run_as_user_snapshot), sqlc.arg(run_as_group_snapshot), sqlc.arg(work_directory_snapshot),
        sqlc.narg(requested_user_id), sqlc.arg(requested_username))
RETURNING id;

-- name: ClaimAutomationJob :execrows
UPDATE automation_execution_job
SET status = 'running', start_time = sqlc.arg(start_time), result_summary = sqlc.arg(result_summary),
    update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id) AND status = 'pending';

-- 取消要算 duration，而 TIMESTAMPDIFF 是 MySQL 方言函数（PG 用 EXTRACT(EPOCH ...)）：
-- 读回 start_time 后在应用层算，别让时长计算把这条语句变成方言分叉。
-- name: GetAutomationJobStartTime :one
SELECT start_time FROM automation_execution_job WHERE id = sqlc.arg(id);

-- name: CancelAutomationJob :execrows
UPDATE automation_execution_job
SET status = 'cancelled', start_time = COALESCE(start_time, sqlc.arg(start_time)), end_time = sqlc.arg(end_time),
    duration_seconds = sqlc.arg(duration_seconds), result_summary = sqlc.arg(result_summary),
    update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id) AND status IN ('pending', 'running');

-- name: FinishAutomationJob :execrows
UPDATE automation_execution_job
SET status = sqlc.arg(status), end_time = sqlc.arg(end_time), duration_seconds = sqlc.arg(duration_seconds),
    result_summary = sqlc.arg(result_summary), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id) AND status <> 'cancelled';

-- 作业实时输出块（运行期间才存在，结束时由执行方删除）：
-- name: InsertAutomationJobLogChunk :exec
INSERT INTO automation_execution_job_log (remark, create_time, update_time, job_id, content)
VALUES (NULL, sqlc.arg(create_time), sqlc.arg(update_time), sqlc.arg(job_id), sqlc.arg(content));

-- name: ListAutomationJobLogChunks :many
SELECT content FROM automation_execution_job_log WHERE job_id = sqlc.arg(job_id) ORDER BY id;

-- name: DeleteAutomationJobLogChunks :exec
DELETE FROM automation_execution_job_log WHERE job_id = sqlc.arg(job_id);

-- 对账用：仍在 running 的作业（含各自超时与开始时间），由对账循环判断是否已失联。
-- 超时在任务表上、作业行只存 task_id，口径与 GetJobTyped 一致（无任务时回落 600s）。
-- name: ListRunningAutomationJobs :many
SELECT j.id, j.start_time, COALESCE(t.execution_timeout_seconds, 600) AS execution_timeout_seconds
FROM automation_execution_job j
LEFT JOIN automation_task t ON t.id = j.task_id
WHERE j.status = 'running' AND j.start_time IS NOT NULL;

-- 失联作业置失败。带 status='running' 守卫：正常收尾（finishJob）可能同时在写，
-- 没有守卫会把刚成功的作业改写成失败。
-- name: FailStaleAutomationJob :execrows
UPDATE automation_execution_job
SET status = 'failed', end_time = sqlc.arg(end_time), duration_seconds = sqlc.arg(duration_seconds),
    result_summary = sqlc.arg(result_summary), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id) AND status = 'running';

-- name: ListAutomationJobHostLogs :many
SELECT host_id_snapshot, host_ip_snapshot, status, agent_job_id, stdout, stderr, error_message
FROM automation_execution_host_log WHERE job_id = sqlc.arg(job_id) ORDER BY id;

-- name: CreateAutomationJobHostLog :exec
INSERT INTO automation_execution_host_log(create_time, update_time, remark, job_id, host_id, host_id_snapshot,
                                          host_name_snapshot, host_ip_snapshot, agent_job_id, status, exit_code,
                                          stdout, stderr, error_message, result_data)
VALUES (sqlc.arg(create_time), sqlc.arg(update_time), NULL, sqlc.arg(job_id), sqlc.narg(host_id),
        sqlc.narg(host_id_snapshot), sqlc.arg(host_name_snapshot), sqlc.arg(host_ip_snapshot), '', sqlc.arg(status),
        sqlc.narg(exit_code), sqlc.arg(stdout), sqlc.arg(stderr), sqlc.arg(error_message), '{}');

-- name: ListAutomationControllerKeysForUpdate :many
SELECT id, public_key, private_key FROM automation_controller_ssh_key FOR UPDATE;

-- name: DeleteAutomationControllerKeys :exec
DELETE FROM automation_controller_ssh_key;

-- name: CreateAutomationControllerKey :exec
INSERT INTO automation_controller_ssh_key(create_time, update_time, remark, public_key, private_key)
VALUES (sqlc.arg(create_time), sqlc.arg(update_time), NULL, sqlc.arg(public_key), sqlc.arg(private_key));

-- 目标主机快照：原实现按主机 ID 个数拼 `IN (?,?,...)`。
-- name: ListAutomationInventoryHosts :many
SELECT h.id, COALESCE(h.instance_name, '') AS instance_name, COALESCE(h.ip, '') AS ip, h.group_id,
       COALESCE(g.name, '') AS group_name, h.agent_online
FROM assets_host h
LEFT JOIN assets_hostgroup g ON g.id = h.group_id
WHERE h.id = ANY(sqlc.arg(host_ids)::bigint[]) AND h.ip IS NOT NULL
ORDER BY h.id;

-- name: ListAutomationHostAgentIdentities :many
SELECT id, COALESCE(instance_name, '') AS instance_name
FROM assets_host WHERE id = ANY(sqlc.arg(host_ids)::bigint[]) ORDER BY id;

-- name: CountAutomationInventoryHosts :one
SELECT COUNT(*) AS existing,
       COUNT(CASE WHEN ip IS NOT NULL THEN 1 END) AS resolved,
       COUNT(DISTINCT group_id) AS group_count
FROM assets_host WHERE id = ANY(sqlc.arg(host_ids)::bigint[]);
