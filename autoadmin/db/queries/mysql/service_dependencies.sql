-- name: ListServiceDependencyTypes :many
SELECT * FROM assets_service_dependency_type
WHERE (code LIKE sqlc.arg(pattern) OR name LIKE sqlc.arg(pattern) OR sqlc.arg(pattern)='')
AND (enabled=sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
ORDER BY id LIMIT ? OFFSET ?;

-- name: CountServiceDependencyTypes :one
SELECT COUNT(*) FROM assets_service_dependency_type
WHERE (code LIKE sqlc.arg(pattern) OR name LIKE sqlc.arg(pattern) OR sqlc.arg(pattern)='')
AND (enabled=sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL);

-- name: GetServiceDependencyType :one
SELECT * FROM assets_service_dependency_type WHERE id=sqlc.arg(id);

-- name: LockServiceDependencyType :one
SELECT * FROM assets_service_dependency_type WHERE id=sqlc.arg(id) FOR UPDATE;

-- name: CreateServiceDependencyType :execlastid
INSERT INTO assets_service_dependency_type (create_time,update_time,code,name,enabled,description)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.arg(code),sqlc.arg(name),sqlc.arg(enabled),sqlc.arg(description));

-- name: UpdateServiceDependencyType :exec
UPDATE assets_service_dependency_type SET update_time=sqlc.arg(update_time),code=sqlc.arg(code),
name=sqlc.arg(name),enabled=sqlc.arg(enabled),description=sqlc.arg(description) WHERE id=sqlc.arg(id);

-- name: CountServiceDependenciesByType :one
SELECT COUNT(*) FROM assets_service_dependency WHERE interaction_type_id=sqlc.arg(id);

-- name: DeleteServiceDependencyType :exec
DELETE FROM assets_service_dependency_type WHERE id=sqlc.arg(id);

-- name: GetServiceDependencyContext :one
SELECT s.id,s.business_system_id,s.environment_id,b.project_id
FROM assets_application_service s JOIN assets_business_system b ON b.id=s.business_system_id
WHERE s.id=sqlc.arg(id) FOR UPDATE;

-- name: LockServiceDependencyDeletionTombstone :one
SELECT service_id,project_code,business_system_code,environment_code,service_code
FROM assets_service_deletion WHERE service_id=sqlc.arg(service_id) FOR UPDATE;

-- name: CountServiceDependenciesForService :one
SELECT COUNT(*) FROM assets_service_dependency WHERE source_service_id=sqlc.arg(source_service_id) OR target_service_id=sqlc.arg(target_service_id);

-- name: CreateServiceDependency :execlastid
INSERT INTO assets_service_dependency (create_time,update_time,source_service_id,target_service_id,
source_business_id,target_business_id,project_id,environment_id,interaction_type_id,strength,protocol,function_scope,failure_impact,fallback_plan)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.arg(source_service_id),sqlc.arg(target_service_id),
sqlc.arg(source_business_id),sqlc.arg(target_business_id),sqlc.arg(project_id),sqlc.arg(environment_id),
sqlc.arg(interaction_type_id),sqlc.arg(strength),sqlc.arg(protocol),sqlc.arg(function_scope),sqlc.arg(failure_impact),sqlc.arg(fallback_plan));

-- name: UpdateServiceDependency :exec
UPDATE assets_service_dependency SET update_time=sqlc.arg(update_time),
source_service_id=sqlc.arg(source_service_id),target_service_id=sqlc.arg(target_service_id),
source_business_id=sqlc.arg(source_business_id),target_business_id=sqlc.arg(target_business_id),
project_id=sqlc.arg(project_id),environment_id=sqlc.arg(environment_id),
interaction_type_id=sqlc.arg(interaction_type_id),strength=sqlc.arg(strength),protocol=sqlc.arg(protocol),
function_scope=sqlc.arg(function_scope),failure_impact=sqlc.arg(failure_impact),fallback_plan=sqlc.arg(fallback_plan)
WHERE id=sqlc.arg(id);

-- name: LockServiceDependency :one
SELECT * FROM assets_service_dependency WHERE id=sqlc.arg(id) FOR UPDATE;

-- name: GetServiceDependency :one
SELECT d.*,s.name AS source_name,t.name AS target_name,k.code AS interaction_type_code,k.name AS interaction_type_name,k.enabled AS interaction_type_enabled
FROM assets_service_dependency d
JOIN assets_application_service s ON s.id=d.source_service_id
JOIN assets_application_service t ON t.id=d.target_service_id
JOIN assets_service_dependency_type k ON k.id=d.interaction_type_id WHERE d.id=sqlc.arg(id);

-- name: ListServiceDependencies :many
SELECT d.*,s.name AS source_name,t.name AS target_name,k.code AS interaction_type_code,k.name AS interaction_type_name,k.enabled AS interaction_type_enabled
FROM assets_service_dependency d
JOIN assets_application_service s ON s.id=d.source_service_id
JOIN assets_application_service t ON t.id=d.target_service_id
JOIN assets_service_dependency_type k ON k.id=d.interaction_type_id
WHERE (d.project_id=sqlc.narg(project_id) OR sqlc.narg(project_id) IS NULL)
AND (d.environment_id=sqlc.narg(environment_id) OR sqlc.narg(environment_id) IS NULL)
AND (d.source_business_id=sqlc.narg(business_id) OR d.target_business_id=sqlc.narg(business_id) OR sqlc.narg(business_id) IS NULL)
AND (d.source_service_id=sqlc.narg(service_id) OR d.target_service_id=sqlc.narg(service_id) OR sqlc.narg(service_id) IS NULL)
AND (d.source_service_id=sqlc.narg(source_service_id) OR sqlc.narg(source_service_id) IS NULL)
AND (d.target_service_id=sqlc.narg(target_service_id) OR sqlc.narg(target_service_id) IS NULL)
AND (d.interaction_type_id=sqlc.narg(interaction_type_id) OR sqlc.narg(interaction_type_id) IS NULL)
AND (d.strength=sqlc.narg(strength) OR sqlc.narg(strength) IS NULL)
AND (s.name LIKE sqlc.arg(pattern) OR t.name LIKE sqlc.arg(pattern) OR d.function_scope LIKE sqlc.arg(pattern) OR d.protocol LIKE sqlc.arg(pattern) OR sqlc.arg(pattern)='')
ORDER BY d.id DESC LIMIT ? OFFSET ?;

-- name: CountServiceDependencies :one
SELECT COUNT(*) FROM assets_service_dependency d
JOIN assets_application_service s ON s.id=d.source_service_id
JOIN assets_application_service t ON t.id=d.target_service_id
WHERE (d.project_id=sqlc.narg(project_id) OR sqlc.narg(project_id) IS NULL)
AND (d.environment_id=sqlc.narg(environment_id) OR sqlc.narg(environment_id) IS NULL)
AND (d.source_business_id=sqlc.narg(business_id) OR d.target_business_id=sqlc.narg(business_id) OR sqlc.narg(business_id) IS NULL)
AND (d.source_service_id=sqlc.narg(service_id) OR d.target_service_id=sqlc.narg(service_id) OR sqlc.narg(service_id) IS NULL)
AND (d.source_service_id=sqlc.narg(source_service_id) OR sqlc.narg(source_service_id) IS NULL)
AND (d.target_service_id=sqlc.narg(target_service_id) OR sqlc.narg(target_service_id) IS NULL)
AND (d.interaction_type_id=sqlc.narg(interaction_type_id) OR sqlc.narg(interaction_type_id) IS NULL)
AND (d.strength=sqlc.narg(strength) OR sqlc.narg(strength) IS NULL)
AND (s.name LIKE sqlc.arg(pattern) OR t.name LIKE sqlc.arg(pattern) OR d.function_scope LIKE sqlc.arg(pattern) OR d.protocol LIKE sqlc.arg(pattern) OR sqlc.arg(pattern)='');

-- name: DeleteServiceDependency :exec
DELETE FROM assets_service_dependency WHERE id=sqlc.arg(id);

-- name: ListDependencyTopologyNodes :many
SELECT s.id,s.name,s.code,s.enabled,s.business_system_id,b.name AS business_system_name
FROM assets_application_service s JOIN assets_business_system b ON b.id=s.business_system_id
WHERE b.project_id=sqlc.arg(project_id) AND s.environment_id=sqlc.arg(environment_id)
ORDER BY s.id LIMIT 1001;

-- name: ListDependencyTopologyInstances :many
SELECT m.service_id,d.id,d.instance_name,d.host_id,h.ip AS host_ip,
d.runtime_status,d.runtime_status_output,d.last_status_check_time,d.ha_role
FROM assets_application_service_deployment m
JOIN assets_application_service s ON s.id=m.service_id
JOIN assets_business_system b ON b.id=s.business_system_id
JOIN assets_application_deployment d ON d.id=m.deployment_id
JOIN assets_host h ON h.id=d.host_id
WHERE b.project_id=sqlc.arg(project_id) AND s.environment_id=sqlc.arg(environment_id)
AND m.enabled=TRUE AND d.enabled=TRUE
ORDER BY m.service_id,d.id LIMIT 10001;
