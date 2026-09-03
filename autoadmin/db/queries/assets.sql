-- name: CountProjects :one
SELECT COUNT(*) FROM assets_project
WHERE name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR code LIKE CAST(sqlc.arg(pattern) AS CHAR) OR owner LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR);

-- name: ListProjects :many
-- business_system_names/business_system_ids 用 '||' 聚合（项目名/系统名可能含逗号），Go 侧拆分为数组。
SELECT p.id, p.create_time, p.update_time, p.remark, p.name, p.code, p.owner, p.enabled,
       GROUP_CONCAT(bs.name ORDER BY bs.id SEPARATOR '||') AS business_system_names,
       GROUP_CONCAT(bs.id ORDER BY bs.id SEPARATOR '||') AS business_system_ids
FROM assets_project p
LEFT JOIN assets_business_system bs ON bs.project_id = p.id
WHERE p.name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR p.code LIKE CAST(sqlc.arg(pattern) AS CHAR) OR p.owner LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(p.remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR)
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
WHERE s.name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR s.code LIKE CAST(sqlc.arg(pattern) AS CHAR) OR s.owner LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(s.remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(p.name, '') LIKE CAST(sqlc.arg(pattern) AS CHAR);

-- name: ListBusinessSystems :many
SELECT s.*, COALESCE(p.name, '') AS project_name, COALESCE(p.code, '') AS project_code
FROM assets_business_system s LEFT JOIN assets_project p ON p.id = s.project_id
WHERE s.name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR s.code LIKE CAST(sqlc.arg(pattern) AS CHAR) OR s.owner LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(s.remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(p.name, '') LIKE CAST(sqlc.arg(pattern) AS CHAR)
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
WHERE name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR code LIKE CAST(sqlc.arg(pattern) AS CHAR) OR owner LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR);

-- name: ListBusinessEnvironments :many
SELECT * FROM assets_business_environment
WHERE name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR code LIKE CAST(sqlc.arg(pattern) AS CHAR) OR owner LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR)
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
WHERE COALESCE(name, '') LIKE CAST(sqlc.arg(pattern) AS CHAR) OR username LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR);

-- name: ListCredentials :many
SELECT * FROM assets_credential
WHERE COALESCE(name, '') LIKE CAST(sqlc.arg(pattern) AS CHAR) OR username LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR)
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
WHERE g.name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(g.remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR);

-- name: ListHostGroups :many
SELECT g.*, COALESCE(p.name, '') AS parent_name,
       (SELECT COUNT(*) FROM assets_host h WHERE h.group_id = g.id) AS host_count
FROM assets_hostgroup g LEFT JOIN assets_hostgroup p ON p.id = g.parent_id
WHERE g.name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(g.remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR)
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
  AND (COALESCE(h.instance_name, '') LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(h.agent_id, '') LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(h.ip, '') LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(h.remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR));

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
  AND (COALESCE(h.instance_name, '') LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(h.agent_id, '') LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(h.ip, '') LIKE CAST(sqlc.arg(pattern) AS CHAR) OR COALESCE(h.remark, '') LIKE CAST(sqlc.arg(pattern) AS CHAR))
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
  webssh_login_users, agent_id, environment_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateHost :exec
UPDATE assets_host SET
  update_time = ?, remark = ?, status = ?, instance_id = ?, ip = ?,
  is_deleted_in_cloud = ?, cloud_account_id = ?, group_id = ?, instance_name = ?,
  collect_status = ?, collect_message = ?, collect_time = ?, agent_online = ?,
  agent_online_time = ?, webssh_default_username = ?, webssh_login_users = ?,
  agent_id = ?, environment_id = ?
WHERE id = ?;

-- name: DeleteHost :exec
DELETE FROM assets_host WHERE id = ?;

-- name: CountApplications :one
SELECT COUNT(*) FROM assets_application
WHERE name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR code LIKE CAST(sqlc.arg(pattern) AS CHAR)
   OR vendor LIKE CAST(sqlc.arg(pattern) AS CHAR) OR description LIKE CAST(sqlc.arg(pattern) AS CHAR);

-- name: ListApplications :many
SELECT a.*,
  (SELECT COUNT(*) FROM assets_application_version v WHERE v.application_id=a.id) AS version_count,
  0 AS deployment_template_count,
  0 AS deployment_count
FROM assets_application a
WHERE a.name LIKE CAST(sqlc.arg(pattern) AS CHAR) OR a.code LIKE CAST(sqlc.arg(pattern) AS CHAR)
   OR a.vendor LIKE CAST(sqlc.arg(pattern) AS CHAR) OR a.description LIKE CAST(sqlc.arg(pattern) AS CHAR)
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
WHERE (sqlc.arg(application_id) IS NULL OR application_id=sqlc.arg(application_id))
  AND version LIKE CAST(sqlc.arg(pattern) AS CHAR);

-- name: ListApplicationVersions :many
SELECT v.*,a.name AS application_name FROM assets_application_version v
JOIN assets_application a ON a.id=v.application_id
WHERE (sqlc.arg(application_id) IS NULL OR v.application_id=sqlc.arg(application_id))
  AND v.version LIKE CAST(sqlc.arg(pattern) AS CHAR)
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
WHERE (sqlc.arg(application_id) IS NULL OR application_id=sqlc.arg(application_id))
  AND name LIKE CAST(sqlc.arg(pattern) AS CHAR);

-- name: ListClusterProfiles :many
SELECT p.*,COALESCE(a.name,'') AS application_name,0 AS service_count FROM assets_cluster_profile p
LEFT JOIN assets_application a ON a.id=p.application_id
WHERE (sqlc.arg(application_id) IS NULL OR p.application_id=sqlc.arg(application_id))
  AND p.name LIKE CAST(sqlc.arg(pattern) AS CHAR)
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
WHERE (? = '' OR t.name LIKE ? OR a.name LIKE ?);

-- name: ListDeploymentTemplates :many
SELECT t.*, a.name AS application_name,
  (SELECT COUNT(*) FROM assets_application_port p WHERE p.deployment_template_id=t.id) AS port_count,
  (SELECT COUNT(*) FROM assets_application_path p WHERE p.deployment_template_id=t.id) AS path_count,
  (SELECT COUNT(*) FROM assets_application_config_file f WHERE f.deployment_template_id=t.id) AS config_file_count,
  (SELECT COUNT(*) FROM assets_application_log_definition l WHERE l.deployment_template_id=t.id) AS log_count,
  (SELECT COUNT(*) FROM assets_application_control_action c WHERE c.deployment_template_id=t.id) AS control_action_count
FROM assets_application_deployment_template t JOIN assets_application a ON a.id=t.application_id
WHERE (? = '' OR t.name LIKE ? OR a.name LIKE ?)
ORDER BY t.application_id, t.id DESC LIMIT ? OFFSET ?;

-- name: GetDeploymentTemplate :one
SELECT t.*, a.name AS application_name,
  (SELECT COUNT(*) FROM assets_application_port p WHERE p.deployment_template_id=t.id) AS port_count,
  (SELECT COUNT(*) FROM assets_application_path p WHERE p.deployment_template_id=t.id) AS path_count,
  (SELECT COUNT(*) FROM assets_application_config_file f WHERE f.deployment_template_id=t.id) AS config_file_count,
  (SELECT COUNT(*) FROM assets_application_log_definition l WHERE l.deployment_template_id=t.id) AS log_count,
  (SELECT COUNT(*) FROM assets_application_control_action c WHERE c.deployment_template_id=t.id) AS control_action_count
FROM assets_application_deployment_template t JOIN assets_application a ON a.id=t.application_id
WHERE t.id=? LIMIT 1;