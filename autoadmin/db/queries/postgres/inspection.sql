-- 本文件由 make derive 从 db/queries/mysql/inspection.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: ListInspectionTargetExecutions :many
SELECT id, deployment_id AS deployment, host_id AS host, target_name, host_id_snapshot,
       host_ip_snapshot, instance_name_snapshot, status, passed, error_message, raw_result,
       start_time, end_time
FROM inspection_target_execution
WHERE execution_id = $1
ORDER BY id;

-- name: GetInspectionGroup :one
SELECT g.id, g.name, g.description, g.enabled, g.category, COALESCE(g.params,'[]') AS params,
       g.application_id AS "application", COALESCE(a.name,'') AS application_name,
       g.create_time, g.update_time
FROM inspection_group g
LEFT JOIN assets_application a ON a.id = g.application_id
WHERE g.id = sqlc.arg(id);

-- name: ListInspectionChecksByGroup :many
SELECT id, name, config, severity, enabled, "order"
FROM inspection_check WHERE group_id = sqlc.arg(group_id) ORDER BY "order", id;

-- name: CountInspectionGroups :one
SELECT COUNT(*) FROM inspection_group
WHERE (name LIKE sqlc.narg(pattern) OR description LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListInspectionGroups :many
SELECT g.id, g.name, g.description, g.enabled, g.category, COALESCE(g.params,'[]') AS params,
       g.application_id AS "application", COALESCE(a.name,'') AS application_name,
       g.create_time, g.update_time
FROM inspection_group g
LEFT JOIN assets_application a ON a.id = g.application_id
WHERE (g.name LIKE sqlc.narg(pattern) OR g.description LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY g.name, g.id
LIMIT $1 OFFSET $2;

-- name: CountInspectionTasks :one
SELECT COUNT(*) FROM inspection_task t
JOIN inspection_group g ON g.id = t.group_id
WHERE (t.name LIKE sqlc.narg(pattern) OR g.name LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListInspectionTasksTyped :many
SELECT t.id, t.name, t.inspection_name, t.group_id AS "group", g.name AS group_name,
       (SELECT COALESCE(json_agg(json_build_object('id', tg2.group_id, 'name', g2.name, 'category', g2.category,
              'mount_type', tg2.mount_type, 'project_id', tg2.project_id, 'environment_id', tg2.environment_id,
              'business_system_id', tg2.business_system_id, 'instance_mode', tg2.instance_mode,
              'service_id', tg2.service_id, 'params', COALESCE(g2.params,'[]'), 'param_values', COALESCE(tg2.param_values,'{}'))), '[]'::json)
        FROM inspection_task_group tg2 JOIN inspection_group g2 ON g2.id = tg2.group_id
        WHERE tg2.task_id = t.id) AS "groups",
       t.concurrency, t.timeout_seconds, t.cron_expression, t.next_run_time,
       t.last_run_time, t.enabled, t.create_time, t.update_time
FROM inspection_task t
JOIN inspection_group g ON g.id = t.group_id
WHERE (t.name LIKE sqlc.narg(pattern) OR g.name LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY t.id DESC
LIMIT $1 OFFSET $2;

-- name: GetInspectionTask :one
SELECT t.id, t.name, t.inspection_name, t.group_id AS "group", g.name AS group_name,
       (SELECT COALESCE(json_agg(json_build_object('id', tg2.group_id, 'name', g2.name, 'category', g2.category,
              'mount_type', tg2.mount_type, 'project_id', tg2.project_id, 'environment_id', tg2.environment_id,
              'business_system_id', tg2.business_system_id, 'instance_mode', tg2.instance_mode,
              'service_id', tg2.service_id, 'params', COALESCE(g2.params,'[]'), 'param_values', COALESCE(tg2.param_values,'{}'))), '[]'::json)
        FROM inspection_task_group tg2 JOIN inspection_group g2 ON g2.id = tg2.group_id
        WHERE tg2.task_id = t.id) AS "groups",
       t.concurrency, t.timeout_seconds, t.cron_expression, t.next_run_time,
       t.last_run_time, t.enabled, t.create_time, t.update_time
FROM inspection_task t
JOIN inspection_group g ON g.id = t.group_id
WHERE t.id = sqlc.arg(id);

-- name: ListInspectionTaskGroupIDs :many
SELECT group_id FROM inspection_task_group WHERE task_id = sqlc.arg(task_id) ORDER BY id;

-- name: ListInspectionTaskBindings :many
SELECT tg.group_id, g.name AS group_name, g.category, g.enabled, COALESCE(g.params,'[]') AS params,
       tg.mount_type, tg.project_id, tg.environment_id, tg.business_system_id, tg.instance_mode, tg.service_id, tg.param_values
FROM inspection_task_group tg
JOIN inspection_group g ON g.id = tg.group_id
WHERE tg.task_id = sqlc.arg(task_id)
ORDER BY tg.id;

-- 挂载点解析：通用组@项目 → 项目下全部实例主机（去重）。
-- name: ListMountProjectHosts :many
SELECT DISTINCT h.id, COALESCE(h.instance_name,'') AS instance_name, COALESCE(h.ip,'') AS ip,
       h.agent_online
FROM assets_host h
JOIN assets_application_deployment d ON d.host_id = h.id
JOIN assets_application_service_deployment l ON l.deployment_id = d.id AND l.enabled = TRUE
JOIN assets_application_service s ON s.id = l.service_id AND s.enabled = TRUE
JOIN assets_business_system b ON b.id = s.business_system_id
WHERE b.project_id = sqlc.arg(project_id) AND h.is_deleted_in_cloud = FALSE
ORDER BY h.id;

-- 挂载点解析：通用组@环境 → 项目×环境下的实例主机（去重）。
-- name: ListMountProjectEnvironmentHosts :many
SELECT DISTINCT h.id, COALESCE(h.instance_name,'') AS instance_name, COALESCE(h.ip,'') AS ip,
       h.agent_online
FROM assets_host h
JOIN assets_application_deployment d ON d.host_id = h.id
JOIN assets_application_service_deployment l ON l.deployment_id = d.id AND l.enabled = TRUE
JOIN assets_application_service s ON s.id = l.service_id AND s.enabled = TRUE
JOIN assets_business_system b ON b.id = s.business_system_id
WHERE b.project_id = sqlc.arg(project_id) AND s.environment_id = sqlc.arg(environment_id)
  AND h.is_deleted_in_cloud = FALSE
ORDER BY h.id;

-- 挂载点解析：应用组@业务(×环境) → 部署实例（每个逻辑服务独立目标，变量各自展开）。
-- name: ListMountBusinessInstances :many
SELECT d.id AS deployment_id, d.host_id, h.id AS host_id2, COALESCE(h.instance_name,'') AS host_name,
       COALESCE(h.ip,'') AS ip, h.agent_online,
       s.id AS service_id, s.name AS service_name, COALESCE(d.instance_name,'') AS instance_name,
       t.app_home, t.run_user, t.work_directory, v.version, s.macro_values, t.macro_definitions
FROM assets_application_service s
JOIN assets_application_service_deployment l ON l.service_id = s.id AND l.enabled = TRUE
JOIN assets_application_deployment d ON d.id = l.deployment_id AND d.enabled = TRUE
JOIN assets_host h ON h.id = d.host_id AND h.is_deleted_in_cloud = FALSE
JOIN assets_application_deployment_template t ON t.id = s.deployment_template_id
JOIN assets_application_version v ON v.id = s.application_version_id
WHERE s.business_system_id = sqlc.arg(business_system_id)
  AND (s.environment_id = sqlc.narg(environment_id) OR sqlc.narg(environment_id) IS NULL)
ORDER BY s.id, d.id;

-- 挂载点解析：应用组@逻辑服务 → 该服务的部署实例（精确绑定）。
-- name: ListMountServiceInstances :many
SELECT d.id AS deployment_id, d.host_id, h.id AS host_id2, COALESCE(h.instance_name,'') AS host_name,
       COALESCE(h.ip,'') AS ip, h.agent_online,
       s.id AS service_id, s.name AS service_name, COALESCE(d.instance_name,'') AS instance_name,
       t.app_home, t.run_user, t.work_directory, v.version, s.macro_values, t.macro_definitions
FROM assets_application_service s
JOIN assets_application_service_deployment l ON l.service_id = s.id AND l.enabled = TRUE
JOIN assets_application_deployment d ON d.id = l.deployment_id AND d.enabled = TRUE
JOIN assets_host h ON h.id = d.host_id AND h.is_deleted_in_cloud = FALSE
JOIN assets_application_deployment_template t ON t.id = s.deployment_template_id
JOIN assets_application_version v ON v.id = s.application_version_id
WHERE s.id = sqlc.arg(service_id)
ORDER BY d.id;

-- name: GetInspectionServiceBusinessChain :one
SELECT s.id AS service_id, b.id AS business_system_id, COALESCE(b.name,'') AS business_system_name,
       COALESCE(b.owner,'') AS business_system_owner, p.id AS project_id, COALESCE(p.name,'') AS project_name,
       COALESCE(p.owner,'') AS project_owner, e.id AS environment_id, COALESCE(e.name,'') AS environment_name
FROM assets_application_service s
LEFT JOIN assets_business_system b ON b.id = s.business_system_id
LEFT JOIN assets_project p ON p.id = b.project_id
LEFT JOIN assets_business_environment e ON e.id = s.environment_id
WHERE s.id = sqlc.arg(service_id);

-- name: ListHostBusinessChains :many
SELECT d.host_id, p.id AS project_id, COALESCE(p.name,'') AS project_name,
       b.id AS business_system_id, COALESCE(b.name,'') AS business_system_name, COALESCE(b.owner,'') AS business_system_owner,
       e.id AS environment_id, COALESCE(e.name,'') AS environment_name
FROM assets_application_deployment d
JOIN assets_application_service_deployment l ON l.deployment_id = d.id AND l.enabled = TRUE
JOIN assets_application_service s ON s.id = l.service_id
LEFT JOIN assets_business_system b ON b.id = s.business_system_id
LEFT JOIN assets_project p ON p.id = b.project_id
LEFT JOIN assets_business_environment e ON e.id = s.environment_id
WHERE d.host_id = ANY(sqlc.arg(host_ids)::bigint[])
GROUP BY d.host_id, p.id, p.name, b.id, b.name, b.owner, e.id, e.name
ORDER BY d.host_id;

-- name: ListEnabledInspectionChecksForRun :many
SELECT name, config, severity, "order"
FROM inspection_check
WHERE group_id = sqlc.arg(group_id) AND enabled = TRUE
ORDER BY "order", id;

-- name: ListHostGroupTreeNodes :many
SELECT id, name, parent_id
FROM assets_hostgroup
ORDER BY name, id;

-- name: CountInspectionExecutions :one
SELECT COUNT(*) FROM inspection_execution e
WHERE (e.task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (e.status = sqlc.narg(status) OR sqlc.narg(status) IS NULL)
  AND (e.trigger_type = sqlc.narg(trigger_type) OR sqlc.narg(trigger_type) IS NULL)
  AND (e.create_time >= sqlc.narg(start_time) OR sqlc.narg(start_time) IS NULL)
  AND (e.create_time <= sqlc.narg(end_time) OR sqlc.narg(end_time) IS NULL);

-- name: ListInspectionExecutions :many
SELECT e.id, e.task_id AS task, COALESCE(t.name,'') AS task_name,
       COALESCE(e.service_snapshot->>'name','') AS target_name,
       e.status, e.trigger_type, e.summary, e.requested_username, e.start_time, e.end_time, e.create_time
FROM inspection_execution e
LEFT JOIN inspection_task t ON t.id = e.task_id
WHERE (e.task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (e.status = sqlc.narg(status) OR sqlc.narg(status) IS NULL)
  AND (e.trigger_type = sqlc.narg(trigger_type) OR sqlc.narg(trigger_type) IS NULL)
  AND (e.create_time >= sqlc.narg(start_time) OR sqlc.narg(start_time) IS NULL)
  AND (e.create_time <= sqlc.narg(end_time) OR sqlc.narg(end_time) IS NULL)
ORDER BY e.id DESC
LIMIT $1 OFFSET $2;

-- name: GetInspectionExecutionTyped :one
SELECT e.id, e.task_id AS task, COALESCE(t.name,'') AS task_name, e.status, e.trigger_type,
       e.task_snapshot, e.group_snapshot, e.service_snapshot, e.target_snapshot, e.summary,
       e.requested_username, e.start_time, e.end_time, e.create_time
FROM inspection_execution e
LEFT JOIN inspection_task t ON t.id = e.task_id
WHERE e.id = sqlc.arg(id);

-- name: ListInspectionResultsByTarget :many
-- expected_value/actual_value 可空，NULL 无法 Scan 进 json.RawMessage，统一回填 JSON null 字面量。
SELECT id, check_key, check_type, name, status, severity, group_id, group_name,
       COALESCE(expected_value, 'null') AS expected_value,
       COALESCE(actual_value, 'null') AS actual_value, message
FROM inspection_result WHERE target_id = sqlc.arg(target_id) ORDER BY id;
-- ---- P2-2：巡检包内联 SQL 的收纳处（调度 / 保留期清理 / 组与任务写路径 / 执行运行期）----
--
-- 两条与内联版本不同的约定：
--  1. 时间一律由应用层传入。`NOW()`/`UTC_TIMESTAMP(6)` 是方言函数，且 PG 的 `now()` 返回
--     timestamptz（落到 timestamp 列会按会话时区换算），跨方言语义不一致（SQL_DESIGN §4.2）。
--  2. PATCH 合并（组的部分更新）在应用层做：先 `FOR UPDATE` 读回现值再整行写，而不是
--     `COALESCE(?, col)`——后者在两侧对可空布尔/JSON 的推导不同，会把签名分歧带进门面。

-- name: ListDueInspectionTasks :many
SELECT id, cron_expression
FROM inspection_task
WHERE enabled = TRUE AND cron_expression <> '' AND next_run_time IS NOT NULL
  AND next_run_time <= sqlc.arg(now)
ORDER BY id;

-- name: ClaimDueInspectionTask :execrows
UPDATE inspection_task
SET next_run_time = sqlc.arg(next_run_time), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id) AND next_run_time <= sqlc.arg(now);

-- 保留期清理：原实现是 MySQL 的多表 DELETE（`DELETE r FROM ... JOIN ...`），PG 不认这个语法，
-- 改成 `WHERE ... IN (子查询)` —— 两方言都接受（子查询查的是别的表，MySQL 的限制不触发）。
-- name: DeleteFinishedInspectionResults :execrows
DELETE FROM inspection_result
WHERE target_id IN (
  SELECT t.id FROM inspection_target_execution t
  JOIN inspection_execution e ON e.id = t.execution_id
  WHERE e.status <> 'pending' AND e.status <> 'running' AND e.end_time < sqlc.arg(cutoff)
);

-- name: DeleteFinishedInspectionTargetExecutions :execrows
DELETE FROM inspection_target_execution
WHERE execution_id IN (
  SELECT e.id FROM inspection_execution e
  WHERE e.status <> 'pending' AND e.status <> 'running' AND e.end_time < sqlc.arg(cutoff)
);

-- name: DeleteFinishedInspectionExecutions :execrows
DELETE FROM inspection_execution
WHERE status <> 'pending' AND status <> 'running' AND end_time < sqlc.arg(cutoff);

-- name: CreateInspectionGroup :one
INSERT INTO inspection_group(name, description, enabled, category, application_id, params, create_time, update_time)
VALUES (sqlc.arg(name), sqlc.arg(description), sqlc.arg(enabled), sqlc.arg(category), sqlc.narg(application_id),
        sqlc.arg(params), sqlc.arg(create_time), sqlc.arg(update_time))
RETURNING id;

-- name: GetInspectionGroupForUpdate :one
SELECT name, description, enabled, category, application_id, params
FROM inspection_group WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: UpdateInspectionGroup :execrows
UPDATE inspection_group
SET name = sqlc.arg(name), description = sqlc.arg(description), enabled = sqlc.arg(enabled),
    category = sqlc.arg(category), application_id = sqlc.narg(application_id), params = sqlc.arg(params),
    update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: DeleteInspectionChecksByGroup :exec
DELETE FROM inspection_check WHERE group_id = sqlc.arg(group_id);

-- name: CreateInspectionCheck :exec
INSERT INTO inspection_check(group_id, name, config, severity, enabled, "order", create_time, update_time)
VALUES (sqlc.arg(group_id), sqlc.arg(name), sqlc.arg(config), sqlc.arg(severity), sqlc.arg(enabled),
        sqlc.arg(check_order), sqlc.arg(create_time), sqlc.arg(update_time));

-- name: CountInspectionTasksByGroup :one
SELECT COUNT(*) FROM inspection_task WHERE group_id = sqlc.arg(group_id);

-- name: DeleteInspectionGroup :execrows
DELETE FROM inspection_group WHERE id = sqlc.arg(id);

-- name: CountApplicationByID :one
SELECT COUNT(*) FROM assets_application WHERE id = sqlc.arg(id);

-- name: CountApplicationServiceByID :one
SELECT COUNT(*) FROM assets_application_service WHERE id = sqlc.arg(id);

-- name: GetInspectionTaskState :one
SELECT name, inspection_name, group_id, concurrency, timeout_seconds, cron_expression, enabled
FROM inspection_task WHERE id = sqlc.arg(id);

-- name: CreateInspectionTask :one
INSERT INTO inspection_task(name, inspection_name, group_id, concurrency, timeout_seconds, cron_expression,
                           next_run_time, last_run_time, enabled, create_time, update_time)
VALUES (sqlc.arg(name), sqlc.arg(inspection_name), sqlc.arg(group_id), sqlc.arg(concurrency),
        sqlc.arg(timeout_seconds), sqlc.arg(cron_expression), sqlc.narg(next_run_time), NULL,
        sqlc.arg(enabled), sqlc.arg(create_time), sqlc.arg(update_time))
RETURNING id;

-- name: UpdateInspectionTask :execrows
UPDATE inspection_task
SET name = sqlc.arg(name), inspection_name = sqlc.arg(inspection_name), group_id = sqlc.arg(group_id),
    concurrency = sqlc.arg(concurrency), timeout_seconds = sqlc.arg(timeout_seconds),
    cron_expression = sqlc.arg(cron_expression), next_run_time = sqlc.narg(next_run_time),
    enabled = sqlc.arg(enabled), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: DeleteInspectionTaskGroups :exec
DELETE FROM inspection_task_group WHERE task_id = sqlc.arg(task_id);

-- name: CreateInspectionTaskGroup :exec
INSERT INTO inspection_task_group(task_id, group_id, mount_type, project_id, environment_id, business_system_id,
                                  service_id, instance_mode, param_values)
VALUES (sqlc.arg(task_id), sqlc.arg(group_id), sqlc.arg(mount_type), sqlc.narg(project_id),
        sqlc.narg(environment_id), sqlc.narg(business_system_id), sqlc.narg(service_id),
        sqlc.narg(instance_mode), sqlc.arg(param_values));

-- name: DetachInspectionExecutionsFromTask :exec
UPDATE inspection_execution SET task_id = NULL, update_time = sqlc.arg(update_time)
WHERE task_id = sqlc.arg(task_id);

-- name: DeleteInspectionTask :execrows
DELETE FROM inspection_task WHERE id = sqlc.arg(id);

-- name: CountInspectionTasksByNameInGroup :one
SELECT COUNT(*) FROM inspection_task
WHERE name = sqlc.arg(name) AND group_id = sqlc.arg(group_id) AND id <> sqlc.arg(exclude_id);

-- name: GetInspectionGroupRunMeta :one
SELECT g.enabled, g.category,
       (SELECT COUNT(*) FROM inspection_check c WHERE c.group_id = g.id AND c.enabled = TRUE) AS enabled_check_count
FROM inspection_group g WHERE g.id = sqlc.arg(id);

-- name: ListInspectionResultsByExecution :many
SELECT r.id, r.target_id, r.check_key, r.check_type, r.name, r.status, r.severity, r.group_id, r.group_name,
       COALESCE(r.expected_value, 'null') AS expected_value,
       COALESCE(r.actual_value, 'null') AS actual_value, r.message
FROM inspection_result r
JOIN inspection_target_execution t ON t.id = r.target_id
WHERE t.execution_id = sqlc.arg(execution_id)
ORDER BY r.target_id, r.id;

-- name: GetInspectionExecutionForUpdate :one
SELECT status, summary FROM inspection_execution WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: CancelInspectionExecution :execrows
UPDATE inspection_execution
SET status = 'canceled', end_time = sqlc.arg(end_time), summary = sqlc.arg(summary), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: CancelRunningInspectionTargets :execrows
UPDATE inspection_target_execution
SET status = 'canceled', end_time = sqlc.arg(end_time), update_time = sqlc.arg(update_time)
WHERE execution_id = sqlc.arg(execution_id) AND status IN ('pending', 'running');

-- name: GetInspectionTaskRunState :one
SELECT id, name, concurrency, timeout_seconds, enabled FROM inspection_task WHERE id = sqlc.arg(id);

-- name: CreateInspectionExecution :one
INSERT INTO inspection_execution(task_id, status, trigger_type, task_snapshot, group_snapshot, service_snapshot,
                                 target_snapshot, summary, requested_user_id, requested_username, start_time, end_time,
                                 create_time, update_time)
VALUES (sqlc.narg(task_id), 'pending', sqlc.arg(trigger_type), sqlc.arg(task_snapshot), sqlc.arg(group_snapshot),
        sqlc.arg(service_snapshot), sqlc.arg(target_snapshot), '{}', sqlc.narg(requested_user_id),
        sqlc.arg(requested_username), NULL, NULL, sqlc.arg(create_time), sqlc.arg(update_time))
RETURNING id;

-- name: CreateInspectionTargetExecution :one
INSERT INTO inspection_target_execution(execution_id, deployment_id, host_id, target_name, host_id_snapshot,
                                        host_ip_snapshot, instance_name_snapshot, status, passed, error_message,
                                        raw_result, start_time, end_time, create_time, update_time)
VALUES (sqlc.arg(execution_id), sqlc.narg(deployment_id), sqlc.narg(host_id), sqlc.arg(target_name),
        sqlc.narg(host_id_snapshot), sqlc.arg(host_ip_snapshot), sqlc.arg(instance_name_snapshot), 'pending',
        NULL, '', '{}', NULL, NULL, sqlc.arg(create_time), sqlc.arg(update_time))
RETURNING id;

-- name: TouchInspectionTaskLastRun :exec
UPDATE inspection_task SET last_run_time = sqlc.arg(last_run_time), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: MarkInspectionExecutionRunning :execrows
UPDATE inspection_execution
SET status = 'running', start_time = sqlc.arg(start_time), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id) AND status = 'pending';

-- 目标结果统计：原实现用 `SUM(status='failed')`（MySQL 把布尔当 0/1，PG 不接受），
-- 换成两方言都认的 `COUNT(CASE WHEN ... THEN 1 END)`，且零行时为 0 而不是 NULL（§4.3）。
-- name: CountInspectionTargetOutcomes :one
SELECT COUNT(CASE WHEN status = 'failed' THEN 1 END) AS failed,
       COUNT(CASE WHEN status = 'success' THEN 1 END) AS success,
       COUNT(CASE WHEN status = 'canceled' THEN 1 END) AS canceled,
       COUNT(CASE WHEN status = 'skipped' THEN 1 END) AS skipped
FROM inspection_target_execution WHERE execution_id = sqlc.arg(execution_id);

-- name: CountInspectionWarningResults :one
SELECT COUNT(*) FROM inspection_result r
JOIN inspection_target_execution t ON t.id = r.target_id
WHERE t.execution_id = sqlc.arg(execution_id) AND r.severity = 'warning' AND r.status NOT IN ('pass', 'skipped');

-- name: FinishInspectionExecution :execrows
UPDATE inspection_execution
SET status = sqlc.arg(status), summary = sqlc.arg(summary), end_time = sqlc.arg(end_time),
    update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: GetApplicationServiceName :one
SELECT name FROM assets_application_service WHERE id = sqlc.arg(id) LIMIT 1;

-- name: GetProjectNameByID :one
SELECT name FROM assets_project WHERE id = sqlc.arg(id) LIMIT 1;

-- name: GetBusinessEnvironmentNameByID :one
SELECT name FROM assets_business_environment WHERE id = sqlc.arg(id) LIMIT 1;

-- name: MarkInspectionTargetRunning :exec
UPDATE inspection_target_execution
SET status = 'running', start_time = sqlc.arg(start_time), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: SkipInspectionTarget :exec
UPDATE inspection_target_execution
SET status = 'skipped', passed = FALSE, error_message = sqlc.arg(error_message),
    end_time = sqlc.arg(end_time), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: CancelInspectionTarget :exec
UPDATE inspection_target_execution
SET status = 'canceled', end_time = sqlc.arg(end_time), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: FinishInspectionTarget :exec
UPDATE inspection_target_execution
SET status = sqlc.arg(status), passed = sqlc.narg(passed), error_message = sqlc.arg(error_message),
    raw_result = sqlc.arg(raw_result), end_time = sqlc.arg(end_time), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- 检查结果落库：原实现按 100 行/批拼多行 INSERT（占位符个数随入参变化，且用 `?`，
-- PG 变体跑不通），改为逐条 sqlc INSERT —— 与 baseline 的 flushResults 同一取舍（见 SQL_DESIGN §6.3）。
-- name: CreateInspectionResult :exec
INSERT INTO inspection_result(target_id, check_key, check_type, name, status, severity, group_id, group_name,
                              expected_value, actual_value, message, create_time, update_time)
VALUES (sqlc.arg(target_id), sqlc.arg(check_key), sqlc.arg(check_type), sqlc.arg(name), sqlc.arg(status),
        sqlc.arg(severity), sqlc.narg(group_id), sqlc.arg(group_name), sqlc.narg(expected_value),
        sqlc.narg(actual_value), sqlc.arg(message), sqlc.arg(create_time), sqlc.arg(update_time));
