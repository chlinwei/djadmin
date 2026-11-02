-- name: LockProjectEnabled :one
SELECT enabled FROM assets_project WHERE id = ? FOR UPDATE;
-- name: UpdateProjectEnabled :exec
UPDATE assets_project SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockBusinessSystemEnabled :one
SELECT enabled FROM assets_business_system WHERE id = ? FOR UPDATE;
-- name: UpdateBusinessSystemEnabled :exec
UPDATE assets_business_system SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockBusinessEnvironmentEnabled :one
SELECT enabled FROM assets_business_environment WHERE id = ? FOR UPDATE;
-- name: UpdateBusinessEnvironmentEnabled :exec
UPDATE assets_business_environment SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockApplicationEnabled :one
SELECT enabled FROM assets_application WHERE id = ? FOR UPDATE;
-- name: UpdateApplicationEnabled :exec
UPDATE assets_application SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockApplicationVersionEnabled :one
SELECT enabled FROM assets_application_version WHERE id = ? FOR UPDATE;
-- name: UpdateApplicationVersionEnabled :exec
UPDATE assets_application_version SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockClusterProfileEnabled :one
SELECT enabled, profile_type FROM assets_cluster_profile WHERE id = ? FOR UPDATE;
-- name: UpdateClusterProfileEnabled :exec
UPDATE assets_cluster_profile SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockDeploymentTemplateEnabled :one
SELECT enabled FROM assets_application_deployment_template WHERE id = ? FOR UPDATE;
-- name: UpdateDeploymentTemplateEnabled :exec
UPDATE assets_application_deployment_template SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockApplicationServiceEnabled :one
SELECT enabled FROM assets_application_service WHERE id = ? FOR UPDATE;
-- name: UpdateApplicationServiceEnabled :exec
UPDATE assets_application_service SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockApplicationDeploymentEnabled :one
SELECT enabled FROM assets_application_deployment WHERE id = ? FOR UPDATE;
-- name: UpdateApplicationDeploymentEnabled :exec
UPDATE assets_application_deployment SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockServiceDependencyTypeEnabled :one
SELECT enabled FROM assets_service_dependency_type WHERE id = ? FOR UPDATE;
-- name: UpdateServiceDependencyTypeEnabled :exec
UPDATE assets_service_dependency_type SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockElasticsearchClusterEnabled :one
SELECT enabled FROM monitor_elasticsearch_cluster WHERE id = ? FOR UPDATE;
-- name: UpdateElasticsearchClusterEnabled :exec
UPDATE monitor_elasticsearch_cluster SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockLogRetentionTierEnabled :one
SELECT enabled FROM monitor_log_retention_tier WHERE id = ? FOR UPDATE;
-- name: UpdateLogRetentionTierEnabled :exec
UPDATE monitor_log_retention_tier SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockLogCollectionFilterRuleEnabled :one
SELECT enabled FROM monitor_log_collection_filter_rule WHERE id = ? FOR UPDATE;
-- name: UpdateLogCollectionFilterRuleEnabled :exec
UPDATE monitor_log_collection_filter_rule SET enabled = ?, update_time = ? WHERE id = ?;

-- name: LockSoftwarePackageEnabled :one
SELECT enabled FROM monitor_software_package WHERE id = ? FOR UPDATE;
-- name: UpdateSoftwarePackageEnabled :exec
UPDATE monitor_software_package SET enabled = ?, update_time = ? WHERE id = ?;
