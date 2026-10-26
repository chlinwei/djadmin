-- name: ListSharedLogSources :many
SELECT x.*, COALESCE(h.instance_name, '') AS host_name, COALESCE(h.ip, '') AS host_ip,
       p.code AS project_code, bs.code AS business_system_code, e.code AS environment_code,
       t.code AS tier_code, r.name AS rule_name, COALESCE(a.code, '') AS application_code,
       r.multiline_enabled, r.start_pattern, r.flush_timeout, r.cluster_id
FROM monitor_shared_log_source x
JOIN assets_host h ON h.id=x.host_id
JOIN assets_business_system bs ON bs.id=x.business_system_id
JOIN assets_project p ON p.id=bs.project_id
JOIN assets_business_environment e ON e.id=x.environment_id
JOIN monitor_log_retention_tier t ON t.id=x.retention_tier_id
JOIN monitor_log_processing_rule r ON r.id=x.processing_rule_id
LEFT JOIN assets_application a ON a.id=r.application_id
ORDER BY x.id;

-- name: ListHostSharedLogSources :many
SELECT x.*, COALESCE(h.instance_name, '') AS host_name, COALESCE(h.ip, '') AS host_ip,
       p.code AS project_code, bs.code AS business_system_code, e.code AS environment_code,
       t.code AS tier_code, r.name AS rule_name, COALESCE(a.code, '') AS application_code,
       r.multiline_enabled, r.start_pattern, r.flush_timeout, r.cluster_id
FROM monitor_shared_log_source x
JOIN assets_host h ON h.id=x.host_id
JOIN assets_business_system bs ON bs.id=x.business_system_id
JOIN assets_project p ON p.id=bs.project_id
JOIN assets_business_environment e ON e.id=x.environment_id
JOIN monitor_log_retention_tier t ON t.id=x.retention_tier_id
JOIN monitor_log_processing_rule r ON r.id=x.processing_rule_id
LEFT JOIN assets_application a ON a.id=r.application_id
WHERE x.host_id IN (sqlc.slice(host_ids))
ORDER BY x.id;

-- name: ListHostSharedLogSourceLinks :many
SELECT l.source_id,l.service_id FROM monitor_shared_log_source_service l
JOIN monitor_shared_log_source x ON x.id=l.source_id
WHERE x.host_id IN (sqlc.slice(host_ids)) ORDER BY l.source_id,l.service_id;

-- name: ListSharedLogSourceLinks :many
SELECT l.source_id, l.service_id, s.name AS service_name, s.code AS service_code
FROM monitor_shared_log_source_service l
JOIN assets_application_service s ON s.id=l.service_id
ORDER BY l.source_id,l.service_id;

-- name: LockSharedLogSource :one
SELECT * FROM monitor_shared_log_source WHERE id=sqlc.arg(id) FOR UPDATE;

-- name: CreateSharedLogSource :execlastid
INSERT INTO monitor_shared_log_source
(name,host_id,business_system_id,environment_id,path_pattern,processing_rule_id,retention_tier_id,include_pattern,exclude_pattern)
VALUES(sqlc.arg(name),sqlc.arg(host_id),sqlc.arg(business_system_id),sqlc.arg(environment_id),sqlc.arg(path_pattern),sqlc.arg(processing_rule_id),sqlc.arg(retention_tier_id),sqlc.arg(include_pattern),sqlc.arg(exclude_pattern));

-- name: UpdateSharedLogSource :exec
UPDATE monitor_shared_log_source SET name=sqlc.arg(name),path_pattern=sqlc.arg(path_pattern),
processing_rule_id=sqlc.arg(processing_rule_id),retention_tier_id=sqlc.arg(retention_tier_id),
include_pattern=sqlc.arg(include_pattern),exclude_pattern=sqlc.arg(exclude_pattern),revision=revision+1
WHERE id=sqlc.arg(id);

-- name: ArchiveSharedLogSource :exec
UPDATE monitor_shared_log_source SET archived=TRUE WHERE id=sqlc.arg(id);

-- name: AdvanceSharedLogSourceGeneration :exec
UPDATE monitor_shared_log_source SET generation=generation+1,revision=revision+1 WHERE id=sqlc.arg(id);

-- name: TouchSharedLogSource :exec
UPDATE monitor_shared_log_source SET revision=revision+1 WHERE id=sqlc.arg(id);

-- name: AddSharedLogSourceLink :exec
INSERT INTO monitor_shared_log_source_service(source_id,service_id) VALUES(sqlc.arg(source_id),sqlc.arg(service_id));

-- name: DeleteSharedLogSourceLinks :exec
DELETE FROM monitor_shared_log_source_service WHERE source_id=sqlc.arg(source_id);

-- name: DeleteSharedLogSourceServiceLink :exec
DELETE FROM monitor_shared_log_source_service WHERE source_id=sqlc.arg(source_id) AND service_id=sqlc.arg(service_id);

-- name: ListSharedLogSourceServiceIDs :many
SELECT service_id FROM monitor_shared_log_source_service WHERE source_id=sqlc.arg(source_id) ORDER BY service_id;

-- name: ListSharedLogServiceSourceIDs :many
SELECT source_id FROM monitor_shared_log_source_service WHERE service_id=sqlc.arg(service_id) ORDER BY source_id;

-- name: CreateSharedLogSourceRevocation :exec
INSERT INTO monitor_shared_log_source_revocation(source_id,generation,last_error)
VALUES(sqlc.arg(source_id),sqlc.arg(generation),'等待撤销采集');

-- name: ListSharedLogSourceRevocations :many
SELECT v.*, COALESCE(h.instance_name, '') AS host_name FROM monitor_shared_log_source_revocation v
JOIN monitor_shared_log_source x ON x.id=v.source_id
JOIN assets_host h ON h.id=x.host_id
ORDER BY v.source_id,v.generation;

-- name: UpdateSharedLogSourceRevocation :exec
UPDATE monitor_shared_log_source_revocation SET stopped=sqlc.arg(stopped),last_error=sqlc.arg(last_error)
WHERE source_id=sqlc.arg(source_id) AND generation=sqlc.arg(generation);

-- name: SharedLogSourceServiceOptions :many
SELECT DISTINCT s.id,s.name,s.business_system_id,s.environment_id,d.host_id
FROM assets_application_service s
JOIN assets_application_service_deployment sd ON sd.service_id=s.id
JOIN assets_application_deployment d ON d.id=sd.deployment_id
WHERE NOT EXISTS (SELECT 1 FROM assets_service_deletion z WHERE z.service_id=s.id)
ORDER BY s.id,d.host_id;

-- name: SharedLogSourceHostOptions :many
SELECT h.id,COALESCE(h.instance_name,'') AS instance_name,COALESCE(h.ip,'') AS ip FROM assets_host h
JOIN monitor_log_collection_target l ON l.host_id=h.id WHERE l.managed_enabled=TRUE ORDER BY h.id;

-- name: SharedLogSourceRuleOptions :many
SELECT r.id,r.name FROM monitor_log_processing_rule r
JOIN monitor_elasticsearch_cluster c ON c.id=r.cluster_id
WHERE c.enabled=TRUE AND c.is_default=TRUE ORDER BY r.name;

-- name: SharedLogSourceTierOptions :many
SELECT id,name,code FROM monitor_log_retention_tier WHERE enabled=TRUE ORDER BY id;

-- name: SharedLogSourceIdentityConflict :one
SELECT COUNT(*) FROM assets_application_service
WHERE business_system_id=sqlc.arg(business_system_id) AND environment_id=sqlc.arg(environment_id)
AND code=sqlc.arg(code);

-- name: SharedLogSourceSearchScope :many
SELECT DISTINCT x.id,p.code AS project_code,bs.code AS business_system_code,e.code AS environment_code,t.code AS tier_code
FROM monitor_shared_log_source x
JOIN monitor_shared_log_source_service l ON l.source_id=x.id
JOIN assets_business_system bs ON bs.id=x.business_system_id
JOIN assets_project p ON p.id=bs.project_id
JOIN assets_business_environment e ON e.id=x.environment_id
JOIN monitor_log_retention_tier t ON t.id=x.retention_tier_id
WHERE l.service_id IN (sqlc.slice(service_ids)) AND x.archived=FALSE
ORDER BY x.id;

-- name: CountSharedLogSourceRuleReferences :one
SELECT COUNT(*) FROM monitor_shared_log_source WHERE processing_rule_id=sqlc.arg(id);

-- name: CountSharedLogSourceTierReferences :one
SELECT COUNT(*) FROM monitor_shared_log_source WHERE retention_tier_id=sqlc.arg(id);

-- name: ListServiceSharedLogRules :many
SELECT DISTINCT r.id AS rule_id,r.name AS rule_name
FROM monitor_shared_log_source x
JOIN monitor_shared_log_source_service l ON l.source_id=x.id
JOIN monitor_log_processing_rule r ON r.id=x.processing_rule_id
WHERE l.service_id=sqlc.arg(service_id) AND x.archived=FALSE ORDER BY r.id;

-- name: ListServiceSharedLogApplyTargets :many
SELECT DISTINCT x.host_id,COALESCE(h.ip,'') AS host_ip,COALESCE(h.instance_name,'') AS host_instance_name,
       l.id AS target_id,COALESCE(l.config_fingerprint,'') AS config_fingerprint,
       COALESCE(l.runtime_status,'') AS runtime_status,COALESCE(l.service_fingerprints,'{}') AS service_fingerprints,
       s.id AS service_id,s.code AS service_code
FROM monitor_shared_log_source_service sl
JOIN assets_application_service s ON s.id=sl.service_id
JOIN monitor_shared_log_source x ON x.id=sl.source_id
JOIN assets_host h ON h.id=x.host_id
LEFT JOIN monitor_log_collection_target l ON l.host_id=x.host_id AND l.managed_enabled=TRUE
WHERE sl.service_id=sqlc.arg(service_id) AND x.archived=FALSE
ORDER BY x.host_id;
