-- 本文件由 make derive 从 db/queries/mysql/assets.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: CountProjects :one
SELECT COUNT(*) FROM assets_project
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(code, '') LIKE sqlc.narg(pattern) OR COALESCE(owner, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern);

-- name: ListProjects :many
-- business_system_names/business_system_ids 用 '||' 聚合（项目名/系统名可能含逗号），Go 侧拆分为数组。
SELECT p.id, p.create_time, p.update_time, p.remark, p.name, p.code, p.owner, p.enabled,
       string_agg(bs.name, '||' ORDER BY bs.id) AS business_system_names,
       string_agg(bs.id::text, '||' ORDER BY bs.id) AS business_system_ids
FROM assets_project p
LEFT JOIN assets_business_system bs ON bs.project_id = p.id
WHERE COALESCE(p.name, '') LIKE sqlc.narg(pattern) OR COALESCE(p.code, '') LIKE sqlc.narg(pattern) OR COALESCE(p.owner, '') LIKE sqlc.narg(pattern) OR COALESCE(p.remark, '') LIKE sqlc.narg(pattern)
GROUP BY p.id
ORDER BY p.name, p.id LIMIT $1 OFFSET $2;

-- name: GetProject :one
SELECT p.id, p.create_time, p.update_time, p.remark, p.name, p.code, p.owner, p.enabled,
       COALESCE((SELECT string_agg(bs.name, '||' ORDER BY bs.id) FROM assets_business_system bs WHERE bs.project_id = p.id), '') AS business_system_names,
       COALESCE((SELECT string_agg(bs.id::text, '||' ORDER BY bs.id) FROM assets_business_system bs WHERE bs.project_id = p.id), '') AS business_system_ids
FROM assets_project p WHERE p.id = $1 LIMIT 1;

-- name: CreateProject :one
INSERT INTO assets_project (create_time, update_time, remark, name, code, owner, enabled) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: UpdateProject :exec
UPDATE assets_project SET update_time = $1, remark = $2, name = $3, code = $4, owner = $5, enabled = $6 WHERE id = $7;

-- name: CountBusinessSystemsByProject :one
SELECT COUNT(*) FROM assets_business_system WHERE project_id = $1;

-- name: DeleteProject :exec
DELETE FROM assets_project WHERE id = $1;

-- name: CountBusinessSystems :one
SELECT COUNT(*) FROM assets_business_system s LEFT JOIN assets_project p ON p.id = s.project_id
WHERE COALESCE(s.name, '') LIKE sqlc.narg(pattern) OR COALESCE(s.code, '') LIKE sqlc.narg(pattern) OR COALESCE(s.owner, '') LIKE sqlc.narg(pattern) OR COALESCE(s.remark, '') LIKE sqlc.narg(pattern) OR COALESCE(p.name, '') LIKE sqlc.narg(pattern);

-- name: ListBusinessSystems :many
SELECT s.*, COALESCE(p.name, '') AS project_name, COALESCE(p.code, '') AS project_code
FROM assets_business_system s LEFT JOIN assets_project p ON p.id = s.project_id
WHERE COALESCE(s.name, '') LIKE sqlc.narg(pattern) OR COALESCE(s.code, '') LIKE sqlc.narg(pattern) OR COALESCE(s.owner, '') LIKE sqlc.narg(pattern) OR COALESCE(s.remark, '') LIKE sqlc.narg(pattern) OR COALESCE(p.name, '') LIKE sqlc.narg(pattern)
ORDER BY s.name, s.id LIMIT $1 OFFSET $2;

-- name: GetBusinessSystem :one
SELECT s.*, COALESCE(p.name, '') AS project_name, COALESCE(p.code, '') AS project_code
FROM assets_business_system s LEFT JOIN assets_project p ON p.id = s.project_id WHERE s.id = $1 LIMIT 1;

-- name: CreateBusinessSystem :one
INSERT INTO assets_business_system (create_time, update_time, remark, name, code, owner, enabled, project_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: UpdateBusinessSystem :exec
UPDATE assets_business_system SET update_time = $1, remark = $2, name = $3, code = $4, owner = $5, enabled = $6, project_id = $7 WHERE id = $8;

-- name: DeleteBusinessSystem :exec
DELETE FROM assets_business_system WHERE id = $1;

-- name: CountBusinessEnvironments :one
SELECT COUNT(*) FROM assets_business_environment
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(code, '') LIKE sqlc.narg(pattern) OR COALESCE(owner, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern);

-- name: ListBusinessEnvironments :many
SELECT * FROM assets_business_environment
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(code, '') LIKE sqlc.narg(pattern) OR COALESCE(owner, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern)
ORDER BY "order", name, id LIMIT $1 OFFSET $2;

-- name: GetBusinessEnvironment :one
SELECT * FROM assets_business_environment WHERE id = $1 LIMIT 1;

-- name: CreateBusinessEnvironment :one
INSERT INTO assets_business_environment (create_time, update_time, remark, name, code, "order", owner, enabled) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: UpdateBusinessEnvironment :exec
UPDATE assets_business_environment SET update_time = $1, remark = $2, name = $3, code = $4, "order" = $5, owner = $6, enabled = $7 WHERE id = $8;

-- name: CountHostsByEnvironment :one
SELECT COUNT(*) FROM assets_host WHERE environment_id = $1;

-- name: DeleteBusinessEnvironment :exec
DELETE FROM assets_business_environment WHERE id = $1;

-- name: CountCredentials :one
SELECT COUNT(*) FROM assets_credential
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(username, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern);

-- name: ListCredentials :many
SELECT * FROM assets_credential
WHERE COALESCE(name, '') LIKE sqlc.narg(pattern) OR COALESCE(username, '') LIKE sqlc.narg(pattern) OR COALESCE(remark, '') LIKE sqlc.narg(pattern)
ORDER BY name, id LIMIT $1 OFFSET $2;

-- name: GetCredential :one
SELECT * FROM assets_credential WHERE id = $1 LIMIT 1;

-- name: CreateCredential :one
INSERT INTO assets_credential (create_time, update_time, remark, name, password, private_key, auth_type, username, port) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: UpdateCredential :exec
UPDATE assets_credential SET update_time = $1, remark = $2, name = $3, password = $4, private_key = $5, auth_type = $6, username = $7, port = $8 WHERE id = $9;

-- name: CountHostCredentialsByCredential :one
SELECT COUNT(*) FROM assets_hostcredential WHERE credential_id = $1;

-- name: DeleteCredential :exec
DELETE FROM assets_credential WHERE id = $1;

-- name: CountHostGroups :one
SELECT COUNT(*) FROM assets_hostgroup g
WHERE COALESCE(g.name, '') LIKE sqlc.narg(pattern) OR COALESCE(g.remark, '') LIKE sqlc.narg(pattern);

-- name: ListHostGroups :many
SELECT g.*, COALESCE(p.name, '') AS parent_name,
       (SELECT COUNT(*) FROM assets_host h WHERE h.group_id = g.id) AS host_count
FROM assets_hostgroup g LEFT JOIN assets_hostgroup p ON p.id = g.parent_id
WHERE COALESCE(g.name, '') LIKE sqlc.narg(pattern) OR COALESCE(g.remark, '') LIKE sqlc.narg(pattern)
ORDER BY g.id LIMIT $1 OFFSET $2;

-- name: GetHostGroup :one
SELECT g.*, COALESCE(p.name, '') AS parent_name,
       (SELECT COUNT(*) FROM assets_host h WHERE h.group_id = g.id) AS host_count
FROM assets_hostgroup g LEFT JOIN assets_hostgroup p ON p.id = g.parent_id WHERE g.id = $1 LIMIT 1;

-- name: ListAllHostGroups :many
SELECT g.*, COALESCE(p.name, '') AS parent_name,
       (SELECT COUNT(*) FROM assets_host h WHERE h.group_id = g.id) AS host_count
FROM assets_hostgroup g LEFT JOIN assets_hostgroup p ON p.id = g.parent_id
ORDER BY g.id;

-- name: CreateHostGroup :one
INSERT INTO assets_hostgroup (create_time, update_time, remark, name, parent_id) VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: UpdateHostGroup :exec
UPDATE assets_hostgroup SET update_time = $1, remark = $2, name = $3, parent_id = $4 WHERE id = $5;

-- name: CountChildHostGroups :one
SELECT COUNT(*) FROM assets_hostgroup WHERE parent_id = $1;

-- name: CountHostsByGroup :one
SELECT COUNT(*) FROM assets_host WHERE group_id = $1;

-- name: DeleteHostGroup :exec
DELETE FROM assets_hostgroup WHERE id = $1;

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
ORDER BY h.id DESC LIMIT $1 OFFSET $2;

-- name: GetHost :one
SELECT h.*, COALESCE(g.name, '') AS group_name, COALESCE(e.name, '') AS environment_name
FROM assets_host h
LEFT JOIN assets_hostgroup g ON g.id = h.group_id
LEFT JOIN assets_business_environment e ON e.id = h.environment_id
WHERE h.id = $1 LIMIT 1;

-- name: CreateHost :one
INSERT INTO assets_host (
  create_time, update_time, remark, status, instance_id, ip, is_deleted_in_cloud,
  cloud_account_id, group_id, instance_name, collect_status, collect_message,
  collect_time, agent_online, agent_online_time, webssh_default_username,
  webssh_login_users, environment_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING id;

-- name: UpdateHost :exec
UPDATE assets_host SET
  update_time = $1, remark = $2, status = $3, instance_id = $4, ip = $5,
  is_deleted_in_cloud = $6, cloud_account_id = $7, group_id = $8, instance_name = $9,
  collect_status = $10, collect_message = $11, collect_time = $12, agent_online = $13,
  agent_online_time = $14, webssh_default_username = $15, webssh_login_users = $16,
  environment_id = $17
WHERE id = $18;

-- name: DeleteHost :exec
DELETE FROM assets_host WHERE id = $1;

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
ORDER BY a.name,a.id LIMIT $1 OFFSET $2;

-- name: GetApplication :one
SELECT a.*,
  (SELECT COUNT(*) FROM assets_application_version v WHERE v.application_id=a.id) AS version_count,
  0 AS deployment_template_count,
  0 AS deployment_count
FROM assets_application a WHERE a.id=$1 LIMIT 1;

-- name: CreateApplication :one
INSERT INTO assets_application(create_time,update_time,remark,name,category,code,description,enabled,vendor)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id;

-- name: UpdateApplication :exec
UPDATE assets_application SET update_time=$1,remark=$2,name=$3,category=$4,code=$5,description=$6,enabled=$7,vendor=$8 WHERE id=$9;

-- name: DeleteApplication :exec
DELETE FROM assets_application WHERE id=$1;

-- name: CountApplicationVersions :one
SELECT COUNT(*) FROM assets_application_version
WHERE (sqlc.arg(application_id) = 0 OR application_id=sqlc.arg(application_id))
  AND COALESCE(version, '') LIKE sqlc.narg(pattern);

-- name: ListApplicationVersions :many
SELECT v.*,a.name AS application_name FROM assets_application_version v
JOIN assets_application a ON a.id=v.application_id
WHERE (sqlc.arg(application_id) = 0 OR v.application_id=sqlc.arg(application_id))
  AND COALESCE(v.version, '') LIKE sqlc.narg(pattern)
ORDER BY v.id DESC LIMIT $1 OFFSET $2;

-- name: GetApplicationVersion :one
SELECT v.*,a.name AS application_name FROM assets_application_version v
JOIN assets_application a ON a.id=v.application_id WHERE v.id=$1 LIMIT 1;

-- name: CreateApplicationVersion :one
INSERT INTO assets_application_version(create_time,update_time,remark,version,release_date,end_of_support,enabled,application_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id;

-- name: UpdateApplicationVersion :exec
UPDATE assets_application_version SET update_time=$1,remark=$2,version=$3,release_date=$4,end_of_support=$5,enabled=$6,application_id=$7 WHERE id=$8;

-- name: DeleteApplicationVersion :exec
DELETE FROM assets_application_version WHERE id=$1;

-- name: CountClusterProfiles :one
SELECT COUNT(*) FROM assets_cluster_profile
WHERE (sqlc.arg(application_id) = 0 OR application_id=sqlc.arg(application_id))
  AND COALESCE(name, '') LIKE sqlc.narg(pattern);

-- name: ListClusterProfiles :many
SELECT p.*,COALESCE(a.name,'') AS application_name,0 AS service_count FROM assets_cluster_profile p
LEFT JOIN assets_application a ON a.id=p.application_id
WHERE (sqlc.arg(application_id) = 0 OR p.application_id=sqlc.arg(application_id))
  AND COALESCE(p.name, '') LIKE sqlc.narg(pattern)
ORDER BY p.id DESC LIMIT $1 OFFSET $2;

-- name: GetClusterProfile :one
SELECT p.*,COALESCE(a.name,'') AS application_name,0 AS service_count FROM assets_cluster_profile p
LEFT JOIN assets_application a ON a.id=p.application_id WHERE p.id=$1 LIMIT 1;

-- name: CreateClusterProfile :one
INSERT INTO assets_cluster_profile(create_time,update_time,remark,name,code,profile_type,enabled,application_id,cluster_type)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id;

-- name: UpdateClusterProfile :exec
UPDATE assets_cluster_profile SET update_time=$1,remark=$2,name=$3,code=$4,profile_type=$5,enabled=$6,application_id=$7,cluster_type=$8 WHERE id=$9;

-- name: DeleteClusterProfile :exec
DELETE FROM assets_cluster_profile WHERE id=$1;

-- name: CountDeploymentTemplates :one
SELECT COUNT(*) FROM assets_application_deployment_template t
JOIN assets_application a ON a.id=t.application_id
WHERE (t.application_id = sqlc.narg(application_id) OR sqlc.narg(application_id) IS NULL)
  AND ($1 = '' OR t.name LIKE $2 OR a.name LIKE $3);

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
  AND ($1 = '' OR t.name LIKE $2 OR a.name LIKE $3)
ORDER BY t.application_id, t.id DESC LIMIT $4 OFFSET $5;

-- name: GetDeploymentTemplate :one
SELECT t.*, a.name AS application_name,
  (SELECT COUNT(*) FROM assets_application_port p WHERE p.deployment_template_id=t.id) AS port_count,
  (SELECT COUNT(*) FROM assets_application_path p WHERE p.deployment_template_id=t.id) AS path_count,
  (SELECT COUNT(*) FROM assets_application_config_file f WHERE f.deployment_template_id=t.id) AS config_file_count,
  (SELECT COUNT(*) FROM assets_application_log_definition l WHERE l.deployment_template_id=t.id) AS log_count,
  (SELECT COUNT(*) FROM assets_application_control_action c WHERE c.deployment_template_id=t.id) AS control_action_count,
  (SELECT COUNT(*) FROM assets_application_service s WHERE s.deployment_template_id=t.id) AS service_count
FROM assets_application_deployment_template t JOIN assets_application a ON a.id=t.application_id
WHERE t.id=$1 LIMIT 1;