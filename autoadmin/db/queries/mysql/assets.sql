-- name: CountProjects :one
SELECT COUNT(*) FROM assets_project
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(code, '') LIKE sqlc.narg(pattern) OR COALESCE(owner, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern);

-- name: ListProjects :many
-- business_system_names/business_system_ids 用 '||' 聚合（项目名/系统名可能含逗号），Go 侧拆分为数组。
SELECT p.id, p.create_time, p.update_time, p.remark, p.name, p.code, p.owner, p.enabled,
       GROUP_CONCAT(bs.name ORDER BY bs.id SEPARATOR '||') AS business_system_names,
       GROUP_CONCAT(bs.id ORDER BY bs.id SEPARATOR '||') AS business_system_ids
FROM assets_project p
LEFT JOIN assets_business_system bs ON bs.project_id = p.id
WHERE COALESCE(p.name, '') LIKE sqlc.narg(pattern) OR COALESCE(p.code, '') LIKE sqlc.narg(pattern) OR COALESCE(p.owner, '') LIKE sqlc.narg(pattern) OR COALESCE(p.remark, '') LIKE sqlc.narg(pattern)
GROUP BY p.id
ORDER BY p.name, p.id LIMIT ? OFFSET ?;

-- name: GetProject :one
SELECT p.id, p.create_time, p.update_time, p.remark, p.name, p.code, p.owner, p.enabled,
       COALESCE((SELECT GROUP_CONCAT(bs.name ORDER BY bs.id SEPARATOR '||') FROM assets_business_system bs WHERE bs.project_id = p.id), '') AS business_system_names,
       COALESCE((SELECT GROUP_CONCAT(bs.id ORDER BY bs.id SEPARATOR '||') FROM assets_business_system bs WHERE bs.project_id = p.id), '') AS business_system_ids
FROM assets_project p WHERE p.id = ? LIMIT 1;

-- name: CreateProject :execresult
INSERT INTO assets_project (create_time, update_time, remark, name, code, owner, enabled) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateProject :exec
UPDATE assets_project SET update_time = ?, remark = ?, name = ?, code = ?, owner = ?, enabled = ? WHERE id = ?;

-- name: CountBusinessSystemsByProject :one
SELECT COUNT(*) FROM assets_business_system WHERE project_id = ?;

-- name: DeleteProject :exec
DELETE FROM assets_project WHERE id = ?;

-- name: CountBusinessSystems :one
SELECT COUNT(*) FROM assets_business_system s LEFT JOIN assets_project p ON p.id = s.project_id
WHERE COALESCE(s.name, '') LIKE sqlc.narg(pattern) OR COALESCE(s.code, '') LIKE sqlc.narg(pattern) OR COALESCE(s.owner, '') LIKE sqlc.narg(pattern) OR COALESCE(s.remark, '') LIKE sqlc.narg(pattern) OR COALESCE(p.name, '') LIKE sqlc.narg(pattern);

-- name: ListBusinessSystems :many
SELECT s.*, COALESCE(p.name, '') AS project_name, COALESCE(p.code, '') AS project_code
FROM assets_business_system s LEFT JOIN assets_project p ON p.id = s.project_id
WHERE COALESCE(s.name, '') LIKE sqlc.narg(pattern) OR COALESCE(s.code, '') LIKE sqlc.narg(pattern) OR COALESCE(s.owner, '') LIKE sqlc.narg(pattern) OR COALESCE(s.remark, '') LIKE sqlc.narg(pattern) OR COALESCE(p.name, '') LIKE sqlc.narg(pattern)
ORDER BY s.name, s.id LIMIT ? OFFSET ?;

-- name: GetBusinessSystem :one
SELECT s.*, COALESCE(p.name, '') AS project_name, COALESCE(p.code, '') AS project_code
FROM assets_business_system s LEFT JOIN assets_project p ON p.id = s.project_id WHERE s.id = ? LIMIT 1;

-- name: CreateBusinessSystem :execresult
INSERT INTO assets_business_system (create_time, update_time, remark, name, code, owner, enabled, project_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateBusinessSystem :exec
UPDATE assets_business_system SET update_time = ?, remark = ?, name = ?, code = ?, owner = ?, enabled = ?, project_id = ? WHERE id = ?;

-- name: DeleteBusinessSystem :exec
DELETE FROM assets_business_system WHERE id = ?;

-- name: CountBusinessEnvironments :one
SELECT COUNT(*) FROM assets_business_environment
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(code, '') LIKE sqlc.narg(pattern) OR COALESCE(owner, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern);

-- name: ListBusinessEnvironments :many
SELECT * FROM assets_business_environment
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(code, '') LIKE sqlc.narg(pattern) OR COALESCE(owner, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern)
ORDER BY `order`, name, id LIMIT ? OFFSET ?;

-- name: GetBusinessEnvironment :one
SELECT * FROM assets_business_environment WHERE id = ? LIMIT 1;

-- name: CreateBusinessEnvironment :execresult
INSERT INTO assets_business_environment (create_time, update_time, remark, name, code, `order`, owner, enabled) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateBusinessEnvironment :exec
UPDATE assets_business_environment SET update_time = ?, remark = ?, name = ?, code = ?, `order` = ?, owner = ?, enabled = ? WHERE id = ?;

-- name: CountHostsByEnvironment :one
SELECT COUNT(*) FROM assets_host WHERE environment_id = ?;

-- name: DeleteBusinessEnvironment :exec
DELETE FROM assets_business_environment WHERE id = ?;

-- name: CountCredentials :one
SELECT COUNT(*) FROM assets_credential
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(username, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern);

-- name: ListCredentials :many
SELECT * FROM assets_credential
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(username, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern)
ORDER BY name, id LIMIT ? OFFSET ?;

-- name: GetCredential :one
SELECT * FROM assets_credential WHERE id = ? LIMIT 1;

-- name: CreateCredential :execresult
INSERT INTO assets_credential (create_time, update_time, remark, name, password, private_key, auth_type, username, port) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateCredential :exec
UPDATE assets_credential SET update_time = ?, remark = ?, name = ?, password = ?, private_key = ?, auth_type = ?, username = ?, port = ? WHERE id = ?;

-- name: CountHostCredentialsByCredential :one
SELECT COUNT(*) FROM assets_hostcredential WHERE credential_id = ?;

-- name: DeleteCredential :exec
DELETE FROM assets_credential WHERE id = ?;

-- name: CountHostGroups :one
SELECT COUNT(*) FROM assets_hostgroup g
WHERE COALESCE(g.name, '') LIKE sqlc.narg(pattern) OR COALESCE(g.remark, '') LIKE sqlc.narg(pattern);

-- name: ListHostGroups :many
SELECT g.*, COALESCE(p.name, '') AS parent_name,
       (SELECT COUNT(*) FROM assets_host h WHERE h.group_id = g.id) AS host_count
FROM assets_hostgroup g LEFT JOIN assets_hostgroup p ON p.id = g.parent_id
WHERE COALESCE(g.name, '') LIKE sqlc.narg(pattern) OR COALESCE(g.remark, '') LIKE sqlc.narg(pattern)
ORDER BY g.id LIMIT ? OFFSET ?;

-- name: GetHostGroup :one
SELECT g.*, COALESCE(p.name, '') AS parent_name,
       (SELECT COUNT(*) FROM assets_host h WHERE h.group_id = g.id) AS host_count
FROM assets_hostgroup g LEFT JOIN assets_hostgroup p ON p.id = g.parent_id WHERE g.id = ? LIMIT 1;

-- name: ListAllHostGroups :many
SELECT g.*, COALESCE(p.name, '') AS parent_name,
       (SELECT COUNT(*) FROM assets_host h WHERE h.group_id = g.id) AS host_count
FROM assets_hostgroup g LEFT JOIN assets_hostgroup p ON p.id = g.parent_id
ORDER BY g.id;

-- name: CreateHostGroup :execresult
INSERT INTO assets_hostgroup (create_time, update_time, remark, name, parent_id) VALUES (?, ?, ?, ?, ?);

-- name: UpdateHostGroup :exec
UPDATE assets_hostgroup SET update_time = ?, remark = ?, name = ?, parent_id = ? WHERE id = ?;

-- name: CountChildHostGroups :one
SELECT COUNT(*) FROM assets_hostgroup WHERE parent_id = ?;

-- name: CountHostsByGroup :one
SELECT COUNT(*) FROM assets_host WHERE group_id = ?;

-- name: DeleteHostGroup :exec
DELETE FROM assets_hostgroup WHERE id = ?;

-- name: CountHosts :one
SELECT COUNT(*) FROM assets_host h
WHERE (sqlc.arg(group_id) = 0 OR h.group_id = sqlc.arg(group_id))
  AND (sqlc.arg(environment_id) = 0 OR h.environment_id = sqlc.arg(environment_id))
  AND (COALESCE(h.instance_name, '') LIKE sqlc.narg(pattern) OR COALESCE(h.ip, '') LIKE sqlc.narg(pattern) OR COALESCE(h.remark, '') LIKE sqlc.narg(pattern));

-- name: ListHosts :many
-- 列表直接带出持久化的系统/硬件快照（与 Django HostListSerializer 的 system/hardware 契约一致），
-- 避免前端靠二阶段采集合并，agent 离线时也有上次采集值可显示。
SELECT h.*, COALESCE(g.name, '') AS group_name, COALESCE(e.name, '') AS environment_name,
       s.hostname AS system_hostname, s.agent_version AS system_agent_version,
       s.os_type AS system_os_type, s.os_version AS system_os_version,
       s.kernel_version AS system_kernel_version,
       hw.cpu_cores, hw.cpu_model, hw.memory_gb, hw.disk_total_gb, hw.architecture
FROM assets_host h
LEFT JOIN assets_hostgroup g ON g.id = h.group_id
LEFT JOIN assets_business_environment e ON e.id = h.environment_id
LEFT JOIN assets_hostsystem s ON s.host_id = h.id
LEFT JOIN assets_hosthardware hw ON hw.host_id = h.id
WHERE (sqlc.arg(group_id) = 0 OR h.group_id = sqlc.arg(group_id))
  AND (sqlc.arg(environment_id) = 0 OR h.environment_id = sqlc.arg(environment_id))
  AND (COALESCE(h.instance_name, '') LIKE sqlc.narg(pattern) OR COALESCE(h.ip, '') LIKE sqlc.narg(pattern) OR COALESCE(h.remark, '') LIKE sqlc.narg(pattern))
ORDER BY h.id DESC LIMIT ? OFFSET ?;

-- name: GetHost :one
SELECT h.*, COALESCE(g.name, '') AS group_name, COALESCE(e.name, '') AS environment_name
FROM assets_host h
LEFT JOIN assets_hostgroup g ON g.id = h.group_id
LEFT JOIN assets_business_environment e ON e.id = h.environment_id
WHERE h.id = ? LIMIT 1;

-- name: CreateHost :execresult
INSERT INTO assets_host (
  create_time, update_time, remark, status, instance_id, ip, is_deleted_in_cloud,
  cloud_account_id, group_id, instance_name, collect_status, collect_message,
  collect_time, agent_online, agent_online_time, webssh_default_username,
  webssh_login_users, environment_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateHost :exec
UPDATE assets_host SET
  update_time = ?, remark = ?, status = ?, instance_id = ?, ip = ?,
  is_deleted_in_cloud = ?, cloud_account_id = ?, group_id = ?, instance_name = ?,
  collect_status = ?, collect_message = ?, collect_time = ?, agent_online = ?,
  agent_online_time = ?, webssh_default_username = ?, webssh_login_users = ?,
  environment_id = ?
WHERE id = ?;

-- name: DeleteHost :exec
DELETE FROM assets_host WHERE id = ?;

-- name: CountApplications :one
SELECT COUNT(*) FROM assets_application
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(code, '') LIKE sqlc.narg(pattern)
   OR COALESCE(vendor, '') LIKE sqlc.narg(pattern) OR COALESCE(description, '') LIKE sqlc.narg(pattern);

-- name: ListApplications :many
SELECT a.*,
  (SELECT COUNT(*) FROM assets_application_version v WHERE v.application_id=a.id) AS version_count,
  0 AS deployment_template_count,
  0 AS deployment_count
FROM assets_application a
WHERE COALESCE(a.name, '') LIKE sqlc.narg(pattern) OR COALESCE(a.code, '') LIKE sqlc.narg(pattern)
   OR COALESCE(a.vendor, '') LIKE sqlc.narg(pattern) OR COALESCE(a.description, '') LIKE sqlc.narg(pattern)
ORDER BY a.name,a.id LIMIT ? OFFSET ?;

-- name: GetApplication :one
SELECT a.*,
  (SELECT COUNT(*) FROM assets_application_version v WHERE v.application_id=a.id) AS version_count,
  0 AS deployment_template_count,
  0 AS deployment_count
FROM assets_application a WHERE a.id=? LIMIT 1;

-- name: CreateApplication :execresult
INSERT INTO assets_application(create_time,update_time,remark,name,category,code,description,enabled,vendor)
VALUES(?,?,?,?,?,?,?,?,?);

-- name: UpdateApplication :exec
UPDATE assets_application SET update_time=?,remark=?,name=?,category=?,code=?,description=?,enabled=?,vendor=? WHERE id=?;

-- name: DeleteApplication :exec
DELETE FROM assets_application WHERE id=?;

-- name: CountApplicationVersions :one
SELECT COUNT(*) FROM assets_application_version
WHERE (sqlc.arg(application_id) = 0 OR application_id=sqlc.arg(application_id))
  AND COALESCE(version, '') LIKE sqlc.narg(pattern);

-- name: ListApplicationVersions :many
SELECT v.*,a.name AS application_name FROM assets_application_version v
JOIN assets_application a ON a.id=v.application_id
WHERE (sqlc.arg(application_id) = 0 OR v.application_id=sqlc.arg(application_id))
  AND COALESCE(v.version, '') LIKE sqlc.narg(pattern)
ORDER BY v.id DESC LIMIT ? OFFSET ?;

-- name: GetApplicationVersion :one
SELECT v.*,a.name AS application_name FROM assets_application_version v
JOIN assets_application a ON a.id=v.application_id WHERE v.id=? LIMIT 1;

-- name: CreateApplicationVersion :execresult
INSERT INTO assets_application_version(create_time,update_time,remark,version,release_date,end_of_support,enabled,application_id)
VALUES(?,?,?,?,?,?,?,?);

-- name: UpdateApplicationVersion :exec
UPDATE assets_application_version SET update_time=?,remark=?,version=?,release_date=?,end_of_support=?,enabled=?,application_id=? WHERE id=?;

-- name: DeleteApplicationVersion :exec
DELETE FROM assets_application_version WHERE id=?;

-- name: CountClusterProfiles :one
SELECT COUNT(*) FROM assets_cluster_profile
WHERE (sqlc.arg(application_id) = 0 OR application_id=sqlc.arg(application_id))
  AND COALESCE(name, '') LIKE sqlc.narg(pattern);

-- name: ListClusterProfiles :many
SELECT p.*,COALESCE(a.name,'') AS application_name,0 AS service_count FROM assets_cluster_profile p
LEFT JOIN assets_application a ON a.id=p.application_id
WHERE (sqlc.arg(application_id) = 0 OR p.application_id=sqlc.arg(application_id))
  AND COALESCE(p.name, '') LIKE sqlc.narg(pattern)
ORDER BY p.id DESC LIMIT ? OFFSET ?;

-- name: GetClusterProfile :one
SELECT p.*,COALESCE(a.name,'') AS application_name,0 AS service_count FROM assets_cluster_profile p
LEFT JOIN assets_application a ON a.id=p.application_id WHERE p.id=? LIMIT 1;

-- name: CreateClusterProfile :execresult
INSERT INTO assets_cluster_profile(create_time,update_time,remark,name,code,profile_type,enabled,application_id,cluster_type)
VALUES(?,?,?,?,?,?,?,?,?);

-- name: UpdateClusterProfile :exec
UPDATE assets_cluster_profile SET update_time=?,remark=?,name=?,code=?,profile_type=?,enabled=?,application_id=?,cluster_type=? WHERE id=?;

-- name: DeleteClusterProfile :exec
DELETE FROM assets_cluster_profile WHERE id=?;

-- name: CountDeploymentTemplates :one
SELECT COUNT(*) FROM assets_application_deployment_template t
JOIN assets_application a ON a.id=t.application_id
WHERE (t.application_id = sqlc.narg(application_id) OR sqlc.narg(application_id) IS NULL)
  AND (? = '' OR t.name LIKE ? OR a.name LIKE ?);

-- name: ListDeploymentTemplates :many
SELECT t.*, a.name AS application_name,
  (SELECT COUNT(*) FROM assets_application_port p WHERE p.deployment_template_id=t.id) AS port_count,
  (SELECT COUNT(*) FROM assets_application_path p WHERE p.deployment_template_id=t.id) AS path_count,
  (SELECT COUNT(*) FROM assets_application_config_file f WHERE f.deployment_template_id=t.id) AS config_file_count,
  (SELECT COUNT(*) FROM assets_application_log_definition l WHERE l.deployment_template_id=t.id) AS log_count,
  (SELECT COUNT(*) FROM assets_application_control_action c WHERE c.deployment_template_id=t.id) AS control_action_count,
  (SELECT COUNT(*) FROM assets_application_service s WHERE s.deployment_template_id=t.id) AS service_count
FROM assets_application_deployment_template t JOIN assets_application a ON a.id=t.application_id
WHERE (t.application_id = sqlc.narg(application_id) OR sqlc.narg(application_id) IS NULL)
  AND (? = '' OR t.name LIKE ? OR a.name LIKE ?)
ORDER BY t.application_id, t.id DESC LIMIT ? OFFSET ?;

-- name: GetDeploymentTemplate :one
SELECT t.*, a.name AS application_name,
  (SELECT COUNT(*) FROM assets_application_port p WHERE p.deployment_template_id=t.id) AS port_count,
  (SELECT COUNT(*) FROM assets_application_path p WHERE p.deployment_template_id=t.id) AS path_count,
  (SELECT COUNT(*) FROM assets_application_config_file f WHERE f.deployment_template_id=t.id) AS config_file_count,
  (SELECT COUNT(*) FROM assets_application_log_definition l WHERE l.deployment_template_id=t.id) AS log_count,
  (SELECT COUNT(*) FROM assets_application_control_action c WHERE c.deployment_template_id=t.id) AS control_action_count,
  (SELECT COUNT(*) FROM assets_application_service s WHERE s.deployment_template_id=t.id) AS service_count
FROM assets_application_deployment_template t JOIN assets_application a ON a.id=t.application_id
WHERE t.id=? LIMIT 1;
-- ---- P2-3：主机域（采集信息落库 / 详情读取 / 身份唯一性校验）----
--
-- 约定与 inspection/automation 一致：时间由应用层传；UPSERT 用 `ON DUPLICATE KEY UPDATE`
-- 并在查询上声明 `-- conflict: <列名>`（派生脚本据此改写成 PG 的 `ON CONFLICT (…) DO UPDATE`；
-- MySQL 的 ON DUPLICATE KEY 对"任意唯一键"生效，语句本身看不出打在哪个键上，必须显式声明）。

-- name: MarkHostCollected :exec
-- collect_time 为 NULL 表示"本次失败、保留上次采集时间"（原实现靠两条 UPDATE 区分）。
UPDATE assets_host
SET collect_status = sqlc.arg(collect_status), collect_message = sqlc.arg(collect_message),
    collect_time = COALESCE(sqlc.narg(collect_time), collect_time), update_time = sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: GetHostRuntimeFingerprint :one
SELECT static_fingerprint FROM assets_hostruntime WHERE host_id = sqlc.arg(host_id) LIMIT 1;

-- name: UpsertHostRuntime :exec
-- conflict: host_id
INSERT INTO assets_hostruntime(create_time,update_time,remark,host_id,cpu_usage_percent,cpu_times,
                              memory_usage_percent,memory,disk_io,os_uptime_seconds,os_boot_time,
                              metrics_sample_window_ms,static_fingerprint,collected_at)
VALUES(sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(host_id),sqlc.narg(cpu_usage_percent),
       sqlc.arg(cpu_times),sqlc.narg(memory_usage_percent),sqlc.arg(memory),sqlc.arg(disk_io),
       sqlc.narg(os_uptime_seconds),sqlc.narg(os_boot_time),sqlc.narg(metrics_sample_window_ms),
       sqlc.arg(static_fingerprint),sqlc.narg(collected_at))
ON DUPLICATE KEY UPDATE update_time=VALUES(update_time),cpu_usage_percent=VALUES(cpu_usage_percent),
  cpu_times=VALUES(cpu_times),memory_usage_percent=VALUES(memory_usage_percent),memory=VALUES(memory),
  disk_io=VALUES(disk_io),os_uptime_seconds=VALUES(os_uptime_seconds),os_boot_time=VALUES(os_boot_time),
  metrics_sample_window_ms=VALUES(metrics_sample_window_ms),static_fingerprint=VALUES(static_fingerprint),
  collected_at=VALUES(collected_at);

-- name: UpsertHostSystem :exec
-- conflict: host_id
INSERT INTO assets_hostsystem(create_time,update_time,remark,host_id,os_type,os_version,os_id,os_id_like,
                              os_version_id,kernel_version,hostname,agent_version,timezone_name,utc_offset,
                              collector_source,collected_at)
VALUES(sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(host_id),sqlc.narg(os_type),
       sqlc.narg(os_version),sqlc.narg(os_id),sqlc.narg(os_id_like),sqlc.narg(os_version_id),
       sqlc.narg(kernel_version),sqlc.narg(hostname),sqlc.narg(agent_version),sqlc.narg(timezone_name),
       sqlc.narg(utc_offset),sqlc.arg(collector_source),sqlc.narg(collected_at))
ON DUPLICATE KEY UPDATE update_time=VALUES(update_time),os_type=VALUES(os_type),os_version=VALUES(os_version),
  os_id=VALUES(os_id),os_id_like=VALUES(os_id_like),os_version_id=VALUES(os_version_id),
  kernel_version=VALUES(kernel_version),hostname=VALUES(hostname),agent_version=VALUES(agent_version),
  timezone_name=VALUES(timezone_name),utc_offset=VALUES(utc_offset),collector_source=VALUES(collector_source),
  collected_at=VALUES(collected_at);

-- name: UpsertHostHardware :exec
-- conflict: host_id
INSERT INTO assets_hosthardware(create_time,update_time,remark,host_id,cpu_cores,cpu_model,memory_gb,
                                disk_total_gb,architecture,collected_at)
VALUES(sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(host_id),sqlc.narg(cpu_cores),
       sqlc.narg(cpu_model),sqlc.narg(memory_gb),sqlc.narg(disk_total_gb),sqlc.narg(architecture),
       sqlc.narg(collected_at))
ON DUPLICATE KEY UPDATE update_time=VALUES(update_time),cpu_cores=VALUES(cpu_cores),cpu_model=VALUES(cpu_model),
  memory_gb=VALUES(memory_gb),disk_total_gb=VALUES(disk_total_gb),architecture=VALUES(architecture),
  collected_at=VALUES(collected_at);

-- 磁盘表没有按 device 的唯一键：整表重建以丢掉已卸载的分区。
-- name: DeleteHostDisks :exec
DELETE FROM assets_hostdisk WHERE host_id = sqlc.arg(host_id);

-- name: CreateHostDisk :exec
INSERT INTO assets_hostdisk(host_id,device,mount_point,size_gb,used_gb,filesystem)
VALUES(sqlc.arg(host_id),sqlc.arg(device),sqlc.narg(mount_point),sqlc.narg(size_gb),sqlc.narg(used_gb),sqlc.narg(filesystem));

-- name: GetHostSystem :one
SELECT os_type,os_version,kernel_version,hostname,agent_version,timezone_name,utc_offset,collector_source
FROM assets_hostsystem WHERE host_id = sqlc.arg(host_id) LIMIT 1;

-- name: GetHostHardware :one
SELECT cpu_cores,cpu_model,memory_gb,disk_total_gb,architecture
FROM assets_hosthardware WHERE host_id = sqlc.arg(host_id) LIMIT 1;

-- name: GetHostRuntime :one
SELECT cpu_usage_percent,cpu_times,memory_usage_percent,memory,disk_io,os_uptime_seconds,os_boot_time,
       metrics_sample_window_ms,collected_at
FROM assets_hostruntime WHERE host_id = sqlc.arg(host_id) LIMIT 1;

-- name: ListHostDisks :many
SELECT device,mount_point,size_gb,used_gb,filesystem FROM assets_hostdisk
WHERE host_id = sqlc.arg(host_id) ORDER BY id;

-- name: ListHostMonitors :many
SELECT id,exporter_type,scrape_port,managed_enabled,install_status,install_message,retry_count,update_time
FROM monitor_target WHERE host_id = sqlc.arg(host_id) ORDER BY id DESC;

-- name: CountOtherHostsByIP :one
SELECT COUNT(*) FROM assets_host WHERE ip = sqlc.arg(ip) AND id <> sqlc.arg(exclude_id);

-- name: CountOtherHostsByInstanceName :one
SELECT COUNT(*) FROM assets_host WHERE instance_name = sqlc.arg(instance_name) AND id <> sqlc.arg(exclude_id);

-- ---- P2-3：安装包 / 应用控制 / 安装模板 ----

-- name: GetDeploymentControlContext :one
SELECT COALESCE(h.instance_name, ''), t.control_type, t.run_user, t.work_directory, t.app_home,
       t.service_name, t.systemd_scope, t.macro_definitions,
       d.instance_name AS deployment_instance_name
FROM assets_application_deployment d
JOIN assets_host h ON h.id = d.host_id
JOIN assets_application_service_deployment l ON l.deployment_id = d.id
JOIN assets_application_service s ON s.id = l.service_id
JOIN assets_application_deployment_template t ON t.id = s.deployment_template_id
WHERE d.id = sqlc.arg(id) LIMIT 1;

-- name: ListDeploymentControlActions :many
SELECT action,command,timeout_seconds,success_exit_codes FROM assets_application_control_action
WHERE deployment_template_id = (
  SELECT deployment_template_id FROM assets_application_service s
  JOIN assets_application_service_deployment l ON l.service_id = s.id
  WHERE l.deployment_id = sqlc.arg(deployment_id) LIMIT 1
);

-- name: UpdateDeploymentRuntimeStatus :exec
UPDATE assets_application_deployment
SET update_time = sqlc.arg(update_time), runtime_status = sqlc.arg(runtime_status),
    runtime_status_output = sqlc.arg(runtime_status_output), last_status_check_time = sqlc.arg(last_status_check_time)
WHERE id = sqlc.arg(id);

-- 单槽位 Agent 安装包：列表接口返回"当前激活包"，下载返回它的文件路径。
-- name: GetActiveAgentPackage :one
SELECT id,file,sha256,size_bytes,is_active,create_time FROM agent_package
WHERE is_active = 1 ORDER BY create_time DESC, id DESC LIMIT 1;

-- name: GetActiveAgentPackageFile :one
SELECT file FROM agent_package WHERE is_active = 1 ORDER BY create_time DESC, id DESC LIMIT 1;

-- name: GetAgentPackage :one
SELECT id,file,sha256,size_bytes,is_active,create_time FROM agent_package WHERE id = sqlc.arg(id);

-- name: GetAgentPackageIDByVersion :one
SELECT id FROM agent_package WHERE version = sqlc.arg(version) LIMIT 1;

-- name: CreateAgentPackage :execlastid
INSERT INTO agent_package(version,file,sha256,size_bytes,is_active,create_time)
VALUES(sqlc.arg(version),sqlc.arg(file),sqlc.arg(sha256),sqlc.arg(size_bytes),1,sqlc.arg(create_time));

-- name: UpdateAgentPackageFile :exec
UPDATE agent_package SET file = sqlc.arg(file), sha256 = sqlc.arg(sha256), size_bytes = sqlc.arg(size_bytes),
       is_active = 1 WHERE id = sqlc.arg(id);

-- name: ActivateAgentPackage :execrows
UPDATE agent_package SET is_active = 1 WHERE id = sqlc.arg(id);

-- name: DeactivateOtherAgentPackages :exec
UPDATE agent_package SET is_active = 0 WHERE id <> sqlc.arg(id) AND is_active = 1;

-- name: DeleteAgentPackage :exec
DELETE FROM agent_package WHERE id = sqlc.arg(id);

-- name: GetAgentInstallPlaybook :one
SELECT content FROM automation_playbook_template WHERE category = sqlc.arg(category) ORDER BY id DESC LIMIT 1;

-- ---- P2-3：Agent 安装/更新流程（作业与主机日志的生命周期、作业创建、活跃任务拦截）----
-- install 与 update 两条流程用的是同一组语句，这里只定义一份，两边共用。
-- 时长（duration_seconds）由应用层算：原实现是 MySQL 的 TIMESTAMPDIFF。

-- name: MarkAgentJobRunning :exec
UPDATE assets_agent_job SET status='running', picked_at=sqlc.arg(picked_at), update_time=sqlc.arg(update_time)
WHERE job_id = sqlc.arg(job_id);

-- name: MarkAgentJobHostLogRunning :exec
UPDATE automation_execution_host_log SET status='running', update_time=sqlc.arg(update_time) WHERE id = sqlc.arg(id);

-- name: FailAgentJob :exec
-- status 由调用方给（失败 'failed' / 超时 'timeout'），其余列语义相同。
UPDATE assets_agent_job
SET status=sqlc.arg(status), error_message=sqlc.arg(error_message), exit_code=sqlc.arg(exit_code),
    stdout=sqlc.arg(stdout), stderr=sqlc.arg(stderr), finished_at=sqlc.narg(finished_at), update_time=sqlc.arg(update_time)
WHERE job_id = sqlc.arg(job_id);

-- name: FailAgentJobHostLog :exec
UPDATE automation_execution_host_log
SET status=sqlc.arg(status), error_message=sqlc.arg(error_message), exit_code=sqlc.arg(exit_code),
    stdout=sqlc.arg(stdout), stderr=sqlc.arg(stderr), update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: UpdateAgentJobStdout :exec
UPDATE assets_agent_job SET stdout=sqlc.arg(stdout), update_time=sqlc.arg(update_time) WHERE job_id = sqlc.arg(job_id);

-- name: UpdateAgentJobHostLogStdout :exec
UPDATE automation_execution_host_log SET stdout=sqlc.arg(stdout), update_time=sqlc.arg(update_time) WHERE id = sqlc.arg(id);

-- name: FinishAgentJob :exec
UPDATE assets_agent_job
SET status=sqlc.arg(status), exit_code=sqlc.arg(exit_code), error_message=sqlc.arg(error_message),
    result_data=sqlc.arg(result_data), finished_at=sqlc.narg(finished_at), update_time=sqlc.arg(update_time)
WHERE job_id = sqlc.arg(job_id);

-- result_data 传 NULL 表示"保持原值"（更新流程的收尾不写 result_data，只有安装流程写）。
-- name: FinishAgentJobHostLog :exec
UPDATE automation_execution_host_log
SET status=sqlc.arg(status), exit_code=sqlc.arg(exit_code), error_message=sqlc.arg(error_message),
    result_data=COALESCE(sqlc.narg(result_data), result_data), update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: CreateAgentExecutionJob :execlastid
INSERT INTO automation_execution_job
  (create_time,update_time,remark,job_id,status,trigger_type,source,inventory_snapshot,extra_vars,result_summary,
   task_name_snapshot,template_name_snapshot,template_content_snapshot,`limit`,run_as_user_snapshot,
   run_as_group_snapshot,work_directory_snapshot,requested_user_id,requested_username,start_time)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(job_id),'running','manual','agent_install',
        sqlc.arg(inventory_snapshot),sqlc.arg(extra_vars),sqlc.arg(result_summary),
        sqlc.arg(task_name_snapshot),sqlc.arg(template_name_snapshot),sqlc.arg(template_content_snapshot),'',
        sqlc.arg(run_as_user_snapshot),sqlc.arg(run_as_group_snapshot),sqlc.arg(work_directory_snapshot),
        sqlc.narg(requested_user_id),sqlc.arg(requested_username),sqlc.narg(start_time));

-- name: CreateAgentJob :exec
INSERT INTO assets_agent_job
  (create_time,update_time,remark,job_id,instance_name,job_type,action,params,timeout_seconds,status,
   result_data,error_message,host_id,exit_code,stderr,stdout)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(job_id),sqlc.arg(instance_name),
        sqlc.arg(job_type),sqlc.arg(action),sqlc.arg(params),sqlc.arg(timeout_seconds),sqlc.arg(status),
        sqlc.arg(result_data),sqlc.arg(error_message),sqlc.narg(host_id),sqlc.arg(exit_code),
        sqlc.arg(stderr),sqlc.arg(stdout));

-- name: CreateAgentJobHostLog :execlastid
INSERT INTO automation_execution_host_log
  (create_time,update_time,remark,host_id_snapshot,host_name_snapshot,host_ip_snapshot,agent_job_id,status,
   exit_code,stdout,stderr,error_message,result_data,host_id,job_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.narg(host_id_snapshot),sqlc.arg(host_name_snapshot),
        sqlc.arg(host_ip_snapshot),sqlc.arg(agent_job_id),sqlc.arg(status),sqlc.arg(exit_code),sqlc.arg(stdout),
        sqlc.arg(stderr),sqlc.arg(error_message),sqlc.arg(result_data),sqlc.narg(host_id),sqlc.arg(job_id));

-- name: ListAgentHostTargets :many
SELECT id, COALESCE(instance_name, '') AS instance_name, COALESCE(ip, '') AS ip
FROM assets_host WHERE id IN (sqlc.slice(host_ids)) ORDER BY id;

-- 同一批主机上"仍在跑"的 Agent 安装任务：先把失联超过 30 秒的标记失败，再拦截活跃的。
-- name: FailStaleAgentInstallJobs :exec
UPDATE assets_agent_job
SET status='failed', error_message='Agent 任务执行进程已失联，请重新提交', exit_code=1,
    finished_at=sqlc.arg(finished_at), update_time=sqlc.arg(update_time)
WHERE host_id IN (sqlc.slice(host_ids)) AND action='install_agent' AND status IN ('queued','running')
  AND update_time < sqlc.arg(stale_before);

-- name: CountActiveAgentInstallJobs :one
SELECT COUNT(*) FROM assets_agent_job
WHERE host_id IN (sqlc.slice(host_ids)) AND action='install_agent' AND status IN ('queued','running');

-- 用户在运行记录中心取消 Agent 安装/更新作业时，同步把对应的 assets_agent_job 与主机日志
-- 置为失败，否则 rejectActiveAgentJobs 会一直拦着"任务执行中"（agent job 与 automation job 是两套状态）。
-- name: CancelAgentJobsByExecution :exec
UPDATE assets_agent_job
SET status='failed', error_message='任务已取消', exit_code=1,
    finished_at=sqlc.arg(finished_at), update_time=sqlc.arg(update_time)
WHERE action='install_agent' AND status IN ('queued','running')
  AND job_id IN (SELECT l.agent_job_id FROM automation_execution_host_log l WHERE l.job_id = sqlc.arg(job_id));

-- name: CancelAgentJobHostLogsByExecution :exec
UPDATE automation_execution_host_log
SET status='failed', error_message='任务已取消', exit_code=1, update_time=sqlc.arg(update_time)
WHERE job_id = sqlc.arg(job_id) AND status IN ('queued','running');

-- name: FinishAgentExecutionJob :exec
UPDATE automation_execution_job
SET status=sqlc.arg(status), end_time=sqlc.narg(end_time), duration_seconds=sqlc.narg(duration_seconds),
    result_summary=sqlc.arg(result_summary), update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- ---- P2-3：部署模板（含嵌套子表）----
-- 原实现的嵌套子表删除是运行时拼表名（`DELETE FROM `+table+` WHERE …`），sqlc 表达不了，
-- 改成每个子表一条显式语句（调用点按表名分派），表名不再是变量。

-- name: CreateDeploymentTemplate :execlastid
INSERT INTO assets_application_deployment_template
  (create_time,update_time,remark,name,control_type,run_user,run_group,app_home,work_directory,service_name,
   ha_system_name,ha_cluster_name,ha_resource_name,enabled,application_id,systemd_scope,macro_definitions)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(name),sqlc.arg(control_type),
        sqlc.arg(run_user),sqlc.arg(run_group),sqlc.arg(app_home),sqlc.arg(work_directory),sqlc.arg(service_name),
        sqlc.arg(ha_system_name),sqlc.arg(ha_cluster_name),sqlc.arg(ha_resource_name),sqlc.arg(enabled),
        sqlc.narg(application_id),sqlc.arg(systemd_scope),sqlc.arg(macro_definitions));

-- name: UpdateDeploymentTemplate :exec
UPDATE assets_application_deployment_template
SET update_time=sqlc.arg(update_time),remark=sqlc.narg(remark),name=sqlc.arg(name),control_type=sqlc.arg(control_type),
    run_user=sqlc.arg(run_user),run_group=sqlc.arg(run_group),app_home=sqlc.arg(app_home),
    work_directory=sqlc.arg(work_directory),service_name=sqlc.arg(service_name),
    ha_system_name=sqlc.arg(ha_system_name),ha_cluster_name=sqlc.arg(ha_cluster_name),
    ha_resource_name=sqlc.arg(ha_resource_name),enabled=sqlc.arg(enabled),application_id=sqlc.narg(application_id),
    systemd_scope=sqlc.arg(systemd_scope),macro_definitions=sqlc.arg(macro_definitions)
WHERE id=sqlc.arg(id);

-- name: DeleteDeploymentTemplate :exec
DELETE FROM assets_application_deployment_template WHERE id=sqlc.arg(id);

-- name: DeleteTemplatePorts :exec
DELETE FROM assets_application_port WHERE deployment_template_id=sqlc.arg(deployment_template_id);

-- name: DeleteTemplatePaths :exec
DELETE FROM assets_application_path WHERE deployment_template_id=sqlc.arg(deployment_template_id);

-- name: DeleteTemplateConfigFiles :exec
DELETE FROM assets_application_config_file WHERE deployment_template_id=sqlc.arg(deployment_template_id);

-- name: DeleteTemplateLogDefinitions :exec
DELETE FROM assets_application_log_definition WHERE deployment_template_id=sqlc.arg(deployment_template_id);

-- name: DeleteTemplateControlActions :exec
DELETE FROM assets_application_control_action WHERE deployment_template_id=sqlc.arg(deployment_template_id);

-- name: DeleteTemplateDockerConfig :exec
DELETE FROM assets_docker_control_config WHERE deployment_template_id=sqlc.arg(deployment_template_id);

-- name: DeleteTemplateComposeConfig :exec
DELETE FROM assets_docker_compose_control_config WHERE deployment_template_id=sqlc.arg(deployment_template_id);

-- name: CreateTemplatePort :exec
INSERT INTO assets_application_port
  (create_time,update_time,remark,name,protocol,bind_address,port,required,external_access,check_enabled,deployment_template_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(name),sqlc.arg(protocol),
        sqlc.arg(bind_address),sqlc.arg(port),sqlc.arg(required),sqlc.arg(external_access),sqlc.arg(check_enabled),
        sqlc.arg(deployment_template_id));

-- name: CreateTemplatePath :exec
INSERT INTO assets_application_path
  (create_time,update_time,remark,name,path_type,path,required,expected_owner,expected_group,expected_mode,check_enabled,deployment_template_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(name),sqlc.arg(path_type),sqlc.arg(path),
        sqlc.arg(required),sqlc.arg(expected_owner),sqlc.arg(expected_group),sqlc.arg(expected_mode),
        sqlc.arg(check_enabled),sqlc.arg(deployment_template_id));

-- name: CreateTemplateConfigFile :exec
INSERT INTO assets_application_config_file
  (create_time,update_time,remark,name,path,file_format,required,deployment_template_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(name),sqlc.arg(path),
        sqlc.arg(file_format),sqlc.arg(required),sqlc.arg(deployment_template_id));

-- name: CreateTemplateLogDefinition :exec
INSERT INTO assets_application_log_definition
  (create_time,update_time,remark,name,path_pattern,deployment_template_id,extra_fields,processing_rule_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(name),sqlc.arg(path_pattern),
        sqlc.arg(deployment_template_id),sqlc.arg(extra_fields),
        sqlc.narg(processing_rule_id));

-- 模板保存按 id 原地更新日志定义（改名/改路径/换规则都保留同一行 → 服务级覆盖行不会失效）。
-- 带 deployment_template_id 条件：请求体混入其它模板的定义 id 时不会误改（影响 0 行）。
-- name: UpdateTemplateLogDefinition :execresult
UPDATE assets_application_log_definition
SET update_time=sqlc.arg(update_time),remark=sqlc.narg(remark),name=sqlc.arg(name),
    path_pattern=sqlc.arg(path_pattern),extra_fields=sqlc.arg(extra_fields),
    processing_rule_id=sqlc.narg(processing_rule_id)
WHERE id=sqlc.arg(id) AND deployment_template_id=sqlc.arg(deployment_template_id);

-- 模板保存时只删"本次未提交"的日志定义（不再整表删重建）。
-- name: DeleteTemplateLogDefinitionsByIDs :exec
DELETE FROM assets_application_log_definition
WHERE deployment_template_id=sqlc.arg(deployment_template_id) AND id IN (sqlc.slice(ids));

-- 删日志定义前先清掉引用它们的服务级覆盖行：外键没有 ON DELETE CASCADE，不清就删不掉；
-- 语义上覆盖行（档位/规则/开关/过滤）依附于该定义，定义没了它也就没有意义。
-- name: DeleteServiceLogSettingsByDefinitionIDs :exec
DELETE FROM assets_application_service_log_setting WHERE log_definition_id IN (sqlc.slice(log_definition_ids));

-- name: CreateTemplateControlAction :exec
INSERT INTO assets_application_control_action
  (create_time,update_time,remark,action,command,timeout_seconds,success_exit_codes,deployment_template_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(action),sqlc.arg(command),
        sqlc.arg(timeout_seconds),sqlc.arg(success_exit_codes),sqlc.arg(deployment_template_id));

-- name: CreateTemplateDockerConfig :exec
INSERT INTO assets_docker_control_config
  (create_time,update_time,remark,container_name,docker_host,expected_image,expected_image_tag,deployment_template_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(container_name),sqlc.arg(docker_host),
        sqlc.arg(expected_image),sqlc.arg(expected_image_tag),sqlc.arg(deployment_template_id));

-- name: CreateTemplateComposeConfig :exec
INSERT INTO assets_docker_compose_control_config
  (create_time,update_time,remark,project_name,service_name,compose_file_path,working_directory,env_file,expected_image,expected_image_tag,deployment_template_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(project_name),sqlc.arg(service_name),
        sqlc.arg(compose_file_path),sqlc.arg(working_directory),sqlc.arg(env_file),sqlc.arg(expected_image),
        sqlc.arg(expected_image_tag),sqlc.arg(deployment_template_id));

-- name: ListTemplatePorts :many
SELECT id,create_time,update_time,remark,name,protocol,bind_address,port,required,external_access,check_enabled
FROM assets_application_port WHERE deployment_template_id=sqlc.arg(deployment_template_id) ORDER BY protocol,port;

-- name: ListTemplatePaths :many
SELECT id,create_time,update_time,remark,name,path_type,path,required,expected_owner,expected_group,expected_mode,check_enabled
FROM assets_application_path WHERE deployment_template_id=sqlc.arg(deployment_template_id) ORDER BY path_type,id;

-- name: ListTemplateConfigFiles :many
SELECT id,create_time,update_time,remark,name,path,file_format,required
FROM assets_application_config_file WHERE deployment_template_id=sqlc.arg(deployment_template_id) ORDER BY id;

-- name: ListTemplateLogDefinitions :many
SELECT id,create_time,update_time,remark,name,path_pattern,extra_fields,processing_rule_id
FROM assets_application_log_definition WHERE deployment_template_id=sqlc.arg(deployment_template_id) ORDER BY id;

-- name: ListTemplateControlActions :many
SELECT id,create_time,update_time,remark,action,command,timeout_seconds,success_exit_codes
FROM assets_application_control_action WHERE deployment_template_id=sqlc.arg(deployment_template_id) ORDER BY id;

-- name: GetTemplateDockerConfig :one
SELECT id,create_time,update_time,remark,container_name,docker_host,expected_image,expected_image_tag
FROM assets_docker_control_config WHERE deployment_template_id=sqlc.arg(deployment_template_id) LIMIT 1;

-- name: GetTemplateComposeConfig :one
SELECT id,create_time,update_time,remark,project_name,service_name,compose_file_path,working_directory,env_file,expected_image,expected_image_tag
FROM assets_docker_compose_control_config WHERE deployment_template_id=sqlc.arg(deployment_template_id) LIMIT 1;

-- ---- P2-3：逻辑服务 / 部署实例（列表过滤、详情、成员关联）+ 逻辑服务的日志设置 --------
-- 原实现的列表过滤是运行时拼 WHERE（搜索 + 可选业务系统/应用过滤 + EXISTS 子查询），
-- 改成 NULL 表示不过滤的 sqlc.narg；`IS NULL` 仍写在 OR 链末尾（SQL_DESIGN §2.2）。

-- name: CountApplicationServices :one
SELECT COUNT(*) FROM assets_application_service s
WHERE (s.name LIKE sqlc.narg(pattern) OR s.code LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
  AND (s.business_system_id = sqlc.narg(business_system_id) OR sqlc.narg(business_system_id) IS NULL);

-- name: ListApplicationServices :many
SELECT s.id,s.create_time,s.update_time,s.remark,s.name,s.code,s.topology_type,s.access_address,s.enabled,
       s.application_id,a.name AS application_name,s.business_system_id,b.name AS business_system_name,
       s.environment_id,COALESCE(e.name,'') AS environment_name,s.application_version_id,
       v.version AS application_version_name,s.deployment_template_id,t.name AS deployment_template_name,
       s.cluster_profile_id,COALESCE(c.name,'') AS cluster_profile_name,s.macro_values,s.log_collection_enabled,
       s.log_retention_tier_id,
       (SELECT COUNT(*) FROM assets_application_service_deployment l WHERE l.service_id=s.id) AS deployment_count
FROM assets_application_service s
JOIN assets_application a ON a.id=s.application_id
JOIN assets_business_system b ON b.id=s.business_system_id
LEFT JOIN assets_business_environment e ON e.id=s.environment_id
JOIN assets_application_version v ON v.id=s.application_version_id
JOIN assets_application_deployment_template t ON t.id=s.deployment_template_id
LEFT JOIN assets_cluster_profile c ON c.id=s.cluster_profile_id
WHERE (s.name LIKE sqlc.narg(pattern) OR s.code LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
  AND (s.business_system_id = sqlc.narg(business_system_id) OR sqlc.narg(business_system_id) IS NULL)
ORDER BY s.business_system_id,s.environment_id,s.name LIMIT ? OFFSET ?;

-- name: GetApplicationServiceDetail :one
SELECT s.id,s.create_time,s.update_time,s.remark,s.name,s.code,s.topology_type,s.access_address,s.enabled,
       s.application_id,a.name AS application_name,s.business_system_id,b.name AS business_system_name,
       s.environment_id,COALESCE(e.name,'') AS environment_name,s.application_version_id,
       v.version AS application_version_name,s.deployment_template_id,t.name AS deployment_template_name,
       s.cluster_profile_id,COALESCE(c.name,'') AS cluster_profile_name,s.macro_values,s.log_collection_enabled,
       s.log_retention_tier_id,
       (SELECT COUNT(*) FROM assets_application_service_deployment l WHERE l.service_id=s.id) AS deployment_count
FROM assets_application_service s
JOIN assets_application a ON a.id=s.application_id
JOIN assets_business_system b ON b.id=s.business_system_id
LEFT JOIN assets_business_environment e ON e.id=s.environment_id
JOIN assets_application_version v ON v.id=s.application_version_id
JOIN assets_application_deployment_template t ON t.id=s.deployment_template_id
LEFT JOIN assets_cluster_profile c ON c.id=s.cluster_profile_id
WHERE s.id=sqlc.arg(id) LIMIT 1;

-- name: ListServiceDeploymentIDs :many
SELECT deployment_id FROM assets_application_service_deployment
WHERE service_id=sqlc.arg(service_id) ORDER BY id;

-- name: CountApplicationDeployments :one
SELECT COUNT(*) FROM assets_application_deployment d
WHERE (EXISTS (SELECT 1 FROM assets_application_service_deployment l
               WHERE l.deployment_id = d.id AND l.service_id = sqlc.narg(service_id))
       OR sqlc.narg(service_id) IS NULL)
  AND (EXISTS (SELECT 1 FROM assets_application_service_deployment l
               JOIN assets_application_service s ON s.id = l.service_id
               WHERE l.deployment_id = d.id AND s.business_system_id = sqlc.narg(business_system_id))
       OR sqlc.narg(business_system_id) IS NULL)
  AND (EXISTS (SELECT 1 FROM assets_application_service_deployment l
               JOIN assets_application_service s ON s.id = l.service_id
               WHERE l.deployment_id = d.id AND s.environment_id = sqlc.narg(environment_id))
       OR sqlc.narg(environment_id) IS NULL);

-- name: ListApplicationDeployments :many
SELECT d.id,d.create_time,d.update_time,d.remark,d.instance_name,d.enabled,d.host_id,
       COALESCE(h.ip,'') AS host_ip,d.runtime_status,d.runtime_status_output,d.last_status_check_time,d.ha_role,
       d.runtime_variables,
       CAST(COALESCE((SELECT s.application_id FROM assets_application_service_deployment l JOIN assets_application_service s ON s.id=l.service_id WHERE l.deployment_id=d.id ORDER BY l.id LIMIT 1), 0) AS SIGNED) AS application_id
FROM assets_application_deployment d
JOIN assets_host h ON h.id=d.host_id
WHERE (EXISTS (SELECT 1 FROM assets_application_service_deployment l
               WHERE l.deployment_id = d.id AND l.service_id = sqlc.narg(service_id))
       OR sqlc.narg(service_id) IS NULL)
  AND (EXISTS (SELECT 1 FROM assets_application_service_deployment l
               JOIN assets_application_service s ON s.id = l.service_id
               WHERE l.deployment_id = d.id AND s.business_system_id = sqlc.narg(business_system_id))
       OR sqlc.narg(business_system_id) IS NULL)
  AND (EXISTS (SELECT 1 FROM assets_application_service_deployment l
               JOIN assets_application_service s ON s.id = l.service_id
               WHERE l.deployment_id = d.id AND s.environment_id = sqlc.narg(environment_id))
       OR sqlc.narg(environment_id) IS NULL)
ORDER BY d.id DESC LIMIT ? OFFSET ?;

-- 按 id 取单个部署实例。列集与 ListApplicationDeployments 完全一致（同一份映射），
-- 供"保存后回读""进详情/控制"这类**按 id** 的场景使用。
--
-- 存在的理由：这些场景原先复用列表查询（`LIMIT 1` 或 `Size: 100000`）在内存里找目标行，
-- 前者恒取 id 最大的一台（编辑任意不是最新的一台都会被误判成"不存在"，见 2026-09-18 的
-- "编辑实例报资产不存在"），后者每次请求要把全部实例拉回来。
-- name: GetApplicationDeploymentDetail :one
SELECT d.id,d.create_time,d.update_time,d.remark,d.instance_name,d.enabled,d.host_id,
       COALESCE(h.ip,'') AS host_ip,d.runtime_status,d.runtime_status_output,d.last_status_check_time,d.ha_role,
       d.runtime_variables,
       CAST(COALESCE((SELECT s.application_id FROM assets_application_service_deployment l JOIN assets_application_service s ON s.id=l.service_id WHERE l.deployment_id=d.id ORDER BY l.id LIMIT 1), 0) AS SIGNED) AS application_id
FROM assets_application_deployment d
JOIN assets_host h ON h.id=d.host_id
WHERE d.id = sqlc.arg(id);

-- name: ListServiceDeploymentLinks :many
SELECT deployment_id,service_id FROM assets_application_service_deployment
WHERE deployment_id IN (sqlc.slice(deployment_ids)) ORDER BY deployment_id,service_id;

-- ---- 逻辑服务写路径 ----

-- name: CreateApplicationService :execlastid
INSERT INTO assets_application_service
  (create_time,update_time,remark,name,code,topology_type,access_address,enabled,application_id,cluster_profile_id,
   environment_id,application_version_id,deployment_template_id,business_system_id,macro_values,
   log_collection_enabled,log_retention_tier_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(name),sqlc.arg(code),
        sqlc.arg(topology_type),sqlc.arg(access_address),sqlc.arg(enabled),sqlc.arg(application_id),
        sqlc.narg(cluster_profile_id),sqlc.narg(environment_id),sqlc.arg(application_version_id),
        sqlc.arg(deployment_template_id),sqlc.arg(business_system_id),sqlc.arg(macro_values),
        sqlc.arg(log_collection_enabled),sqlc.narg(log_retention_tier_id));

-- name: UpdateApplicationService :exec
UPDATE assets_application_service
SET update_time=sqlc.arg(update_time),remark=sqlc.narg(remark),name=sqlc.arg(name),code=sqlc.arg(code),
    topology_type=sqlc.arg(topology_type),access_address=sqlc.arg(access_address),enabled=sqlc.arg(enabled),
    application_id=sqlc.arg(application_id),cluster_profile_id=sqlc.narg(cluster_profile_id),
    environment_id=sqlc.narg(environment_id),application_version_id=sqlc.arg(application_version_id),
    deployment_template_id=sqlc.arg(deployment_template_id),business_system_id=sqlc.arg(business_system_id),
    macro_values=sqlc.arg(macro_values),log_collection_enabled=sqlc.arg(log_collection_enabled),
    log_retention_tier_id=sqlc.narg(log_retention_tier_id)
WHERE id=sqlc.arg(id);

-- name: DeleteApplicationService :exec
DELETE FROM assets_application_service WHERE id=sqlc.arg(id);

-- 服务级日志采集总开关（`log_collection_enabled`）的单独写入。
--
-- 为什么单独一条：它原先只能随整个服务表单（`UpdateApplicationService`，PATCH 校验要求
-- 应用/版本/模板/名称/编码等全字段）一起提交，于是"只想开关采集"必须伪造一份完整表单；
-- 日志中心页的服务级开关需要按一次改一个字段。与按行覆盖值（UpsertServiceLogOverride）
-- 同一思路：粒度最小的写操作要有自己的接口。
--
-- 不动认证状态：采集总开关不进认证指纹（见 log_format_fingerprint.go 的输入清单），
-- 而且认证四列在 log_setting 上、根本不在这一行。
-- 单列读取：log-config 接口要带上服务级采集总开关（页面据此区分"总开关关了"与"逐条关了"）。
-- name: GetApplicationServiceLogCollection :one
SELECT log_collection_enabled FROM assets_application_service WHERE id=sqlc.arg(id);

-- name: UpdateApplicationServiceLogCollection :execresult
UPDATE assets_application_service
SET update_time=sqlc.arg(update_time),log_collection_enabled=sqlc.arg(log_collection_enabled)
WHERE id=sqlc.arg(id);

-- name: DeleteServiceDeployments :exec
DELETE FROM assets_application_service_deployment WHERE service_id=sqlc.arg(service_id);

-- name: CreateServiceDeployment :exec
INSERT INTO assets_application_service_deployment (create_time,update_time,remark,enabled,deployment_id,service_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(enabled),sqlc.arg(deployment_id),sqlc.arg(service_id));

-- name: DeleteServiceLogSettings :exec
DELETE FROM assets_application_service_log_setting WHERE service_id=sqlc.arg(service_id);

-- 服务级日志覆盖：**只有采集开关、保留档位、采集过滤规则**。
-- 解析规则不在这张表里——它只由模板日志定义决定（迁移 000035 删掉了 processing_rule_id）。
-- name: CreateServiceLogSetting :exec
INSERT INTO assets_application_service_log_setting
  (create_time,update_time,remark,collection_enabled,log_definition_id,retention_tier_id,service_id,collection_filter_rule_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.narg(collection_enabled),sqlc.arg(log_definition_id),
        sqlc.narg(retention_tier_id),sqlc.arg(service_id),
        sqlc.narg(collection_filter_rule_id));

-- name: CreateApplicationDeployment :execlastid
INSERT INTO assets_application_deployment
  (create_time,update_time,remark,instance_name,enabled,host_id,runtime_status,runtime_status_output,ha_role,runtime_variables)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(instance_name),sqlc.arg(enabled),
        sqlc.arg(host_id),sqlc.arg(runtime_status),sqlc.arg(runtime_status_output),sqlc.arg(ha_role),
        sqlc.arg(runtime_variables));

-- name: UpdateApplicationDeployment :exec
UPDATE assets_application_deployment
SET update_time=sqlc.arg(update_time),remark=sqlc.narg(remark),instance_name=sqlc.arg(instance_name),
    enabled=sqlc.arg(enabled),host_id=sqlc.arg(host_id),runtime_status=sqlc.arg(runtime_status),
    runtime_status_output=sqlc.arg(runtime_status_output),ha_role=sqlc.arg(ha_role),
    runtime_variables=sqlc.arg(runtime_variables)
WHERE id=sqlc.arg(id);

-- name: DeleteApplicationDeployment :exec
DELETE FROM assets_application_deployment WHERE id=sqlc.arg(id);

-- ---- 逻辑服务的日志设置读取（编辑弹窗"模板日志"表格 = 模板日志定义 + 服务级覆盖）----

-- 模板日志定义 + 服务级覆盖。两条"只归一侧"的字段：
--   - 采集开关只有服务级一处（覆盖列，NULL/无覆盖行=采，见 ListHostLogRenderEntries），
--     模板不再有 collection_enabled（迁移 000034 删列）；
--   - **解析规则只有模板一处**（ld.processing_rule_id，服务侧只读展示），
--     服务不再有 processing_rule_id（迁移 000035 删列）。规则名一并带出来给弹窗只读展示；
--   - 日志格式认证（迁移 000036）：ls.format_verified_* 是上一次认证的结果，
--     与这里同时带出的"指纹输入"（规则 update_time、服务宏、应用版本）在应用层比对，
--     得出 format_state（verified / needs_recheck / unverified）。
-- name: ListServiceTemplateLogs :many
SELECT ld.id, ld.name, ld.path_pattern,
       ld.processing_rule_id, COALESCE(rule.name, '') AS processing_rule_name,
       COALESCE(rule.update_time, ld.create_time) AS processing_rule_update_time,
       s.application_version_id,
       ls.retention_tier_id, ls.collection_enabled AS override_collection_enabled,
       ls.collection_filter_rule_id,
       ls.format_verified_at, ls.format_verified_fingerprint,
       ls.format_verified_source, ls.format_verified_by,
       s.code AS service_code, p.code AS project_code, e.code AS environment_code, bs.code AS business_system_code,
       COALESCE(s.macro_values, '{}') AS macro_values,
       COALESCE(tier.code, (SELECT code FROM monitor_log_retention_tier WHERE is_default = TRUE ORDER BY id LIMIT 1), 'std') AS tier_code
FROM assets_application_log_definition ld
JOIN assets_application_service s ON s.id = sqlc.arg(service_id)
LEFT JOIN assets_application_service_log_setting ls
  ON ls.log_definition_id = ld.id AND ls.service_id = s.id
LEFT JOIN monitor_log_processing_rule rule ON rule.id = ld.processing_rule_id
JOIN assets_business_system bs ON bs.id = s.business_system_id
JOIN assets_project p ON p.id = bs.project_id
LEFT JOIN assets_business_environment e ON e.id = s.environment_id
LEFT JOIN monitor_log_retention_tier tier ON tier.id = COALESCE(ls.retention_tier_id, s.log_retention_tier_id)
WHERE ld.deployment_template_id = s.deployment_template_id
ORDER BY ld.id;

-- 日志格式认证的 instance 依据：一条 (逻辑服务 × 部署实例 × 日志定义) 的取样上下文。
-- 只取"渲染该实例日志文件真实路径"所需的三层宏与实际路径模板：
--   - 服务级 macro_values（模板 macro_definitions 作默认值）
--   - 实例级 runtime_variables + 部署模板 app_home（作为 APP_HOME 默认值）
-- 三层合并与 Filebeat 下发走同一套口径（见 logcollect.resolveMacros / instanceMacros），
-- 所以这里展开出来的路径与主机上真正在采的文件一致。
-- host_id 用于复用 logcollect 的批量渲染装载；host_instance_name 是 agent 会话的路由键
-- （注意是 assets_host.instance_name，不是部署实例自己的 instance_name）。
-- name: GetLogVerifyInstanceContext :one
SELECT d.id AS deployment_id, d.host_id, COALESCE(h.instance_name, '') AS host_instance_name,
       d.instance_name AS deployment_instance_name,
       COALESCE(d.runtime_variables, '{}') AS runtime_variables,
       COALESCE(t.app_home, '') AS app_home,
       COALESCE(t.macro_definitions, '[]') AS macro_definitions,
       COALESCE(s.macro_values, '{}') AS macro_values,
       ld.path_pattern AS path_pattern
FROM assets_application_deployment d
JOIN assets_host h ON h.id = d.host_id
JOIN assets_application_service_deployment sd ON sd.deployment_id = d.id
JOIN assets_application_service s ON s.id = sd.service_id
JOIN assets_application_deployment_template t ON t.id = s.deployment_template_id
JOIN assets_application_log_definition ld ON ld.deployment_template_id = t.id
WHERE d.id = sqlc.arg(deployment_id) AND s.id = sqlc.arg(service_id) AND ld.id = sqlc.arg(log_definition_id)
LIMIT 1;

-- 认证/失效：写回一次"格式认证"的结果（指纹 + 依据 + 操作人）。
-- 由格式认证流程调用（取实例样例或规则样例跑 _simulate 通过后），也用于"人工确认豁免"。
--
-- **必须是 upsert**：认证状态读的是 (服务 × 日志定义)，而覆盖行只在"有覆盖"时才存在
-- （ListServiceTemplateLogs 对 ls 是 LEFT JOIN）。UPDATE-only 对没有覆盖行的组合会静默影响 0 行，
-- 认证结果写不进去、format_state 永远停在 unverified。冲突目标是唯一键
-- unique_service_log_setting(service_id, log_definition_id)；插入行只带认证四列，
-- 覆盖列（collection_enabled / retention_tier_id / collection_filter_rule_id）留 NULL = 不覆盖。
-- name: UpsertLogSettingFormatVerified :exec
-- conflict: service_id, log_definition_id
INSERT INTO assets_application_service_log_setting
  (create_time,update_time,remark,collection_enabled,log_definition_id,retention_tier_id,service_id,
   collection_filter_rule_id,format_verified_at,format_verified_fingerprint,format_verified_source,format_verified_by)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,NULL,sqlc.arg(log_definition_id),NULL,sqlc.arg(service_id),
        NULL,sqlc.arg(format_verified_at),sqlc.arg(format_verified_fingerprint),
        sqlc.arg(format_verified_source),sqlc.arg(format_verified_by))
ON DUPLICATE KEY UPDATE update_time=VALUES(update_time),format_verified_at=VALUES(format_verified_at),
  format_verified_fingerprint=VALUES(format_verified_fingerprint),
  format_verified_source=VALUES(format_verified_source),format_verified_by=VALUES(format_verified_by);

-- name: ListServiceLogSettings :many
SELECT log_definition_id,retention_tier_id,collection_enabled,collection_filter_rule_id
FROM assets_application_service_log_setting WHERE service_id=sqlc.arg(service_id) ORDER BY log_definition_id;

-- 服务级日志覆盖的**按行**写入：只写"这个服务在这条日志上的覆盖值"（采集开关 + 保留档位），
-- 供日志中心页的内联开关/档位用。区别于 `SaveApplicationService` 的整表替换——那条是
-- "提交的集合即全量"（DELETE 后重插），只允许**完整表单**调用；两者边界见
-- docs/architecture/LOG_COLLECTION_ARCHITECTURE.md §9.5。
--
-- 三条必须守住的语义：
--  1. **不碰 format_verified_\***：认证结果只由格式认证流程写（UpsertLogSettingFormatVerified）。
--     改采集开关/档位不改日志格式、也不进认证指纹，所以覆盖值更新后认证状态必须原样保留
--     ——UPDATE 分支只列这两个覆盖列，就是靠这一点保证的。
--  2. narg 传 NULL = "不覆盖"（采集默认采、档位继承服务默认），与覆盖行不存在等价。
--  3. 没有覆盖行时插入一条：插入行的认证四列取默认值，语义是"这个组合从未认证"。
-- 冲突目标是唯一键 unique_service_log_setting(service_id, log_definition_id)。
-- name: UpsertServiceLogOverride :exec
-- conflict: service_id, log_definition_id
INSERT INTO assets_application_service_log_setting
  (create_time,update_time,remark,collection_enabled,log_definition_id,retention_tier_id,service_id,collection_filter_rule_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.narg(collection_enabled),sqlc.arg(log_definition_id),
        sqlc.narg(retention_tier_id),sqlc.arg(service_id),NULL)
ON DUPLICATE KEY UPDATE update_time=VALUES(update_time),collection_enabled=VALUES(collection_enabled),
  retention_tier_id=VALUES(retention_tier_id);
