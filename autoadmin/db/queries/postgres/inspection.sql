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
       t.app_home, t.run_user, t.work_directory, v.version, s.macro_values
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
       t.app_home, t.run_user, t.work_directory, v.version, s.macro_values
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
WHERE d.host_id IN (sqlc.slice(host_ids))
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