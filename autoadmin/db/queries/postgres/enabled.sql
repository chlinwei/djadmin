-- 本文件由 make derive 从 db/queries/mysql/enabled.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: LockProjectEnabled :one
SELECT enabled FROM assets_project WHERE id = $1 FOR UPDATE;
-- name: UpdateProjectEnabled :exec
UPDATE assets_project SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockBusinessSystemEnabled :one
SELECT enabled FROM assets_business_system WHERE id = $1 FOR UPDATE;
-- name: UpdateBusinessSystemEnabled :exec
UPDATE assets_business_system SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockBusinessEnvironmentEnabled :one
SELECT enabled FROM assets_business_environment WHERE id = $1 FOR UPDATE;
-- name: UpdateBusinessEnvironmentEnabled :exec
UPDATE assets_business_environment SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockApplicationEnabled :one
SELECT enabled FROM assets_application WHERE id = $1 FOR UPDATE;
-- name: UpdateApplicationEnabled :exec
UPDATE assets_application SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockApplicationVersionEnabled :one
SELECT enabled FROM assets_application_version WHERE id = $1 FOR UPDATE;
-- name: UpdateApplicationVersionEnabled :exec
UPDATE assets_application_version SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockClusterProfileEnabled :one
SELECT enabled, profile_type FROM assets_cluster_profile WHERE id = $1 FOR UPDATE;
-- name: UpdateClusterProfileEnabled :exec
UPDATE assets_cluster_profile SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockDeploymentTemplateEnabled :one
SELECT enabled FROM assets_application_deployment_template WHERE id = $1 FOR UPDATE;
-- name: UpdateDeploymentTemplateEnabled :exec
UPDATE assets_application_deployment_template SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockApplicationServiceEnabled :one
SELECT enabled FROM assets_application_service WHERE id = $1 FOR UPDATE;
-- name: UpdateApplicationServiceEnabled :exec
UPDATE assets_application_service SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockApplicationDeploymentEnabled :one
SELECT enabled FROM assets_application_deployment WHERE id = $1 FOR UPDATE;
-- name: UpdateApplicationDeploymentEnabled :exec
UPDATE assets_application_deployment SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockServiceDependencyTypeEnabled :one
SELECT enabled FROM assets_service_dependency_type WHERE id = $1 FOR UPDATE;
-- name: UpdateServiceDependencyTypeEnabled :exec
UPDATE assets_service_dependency_type SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockElasticsearchClusterEnabled :one
SELECT enabled FROM monitor_elasticsearch_cluster WHERE id = $1 FOR UPDATE;
-- name: UpdateElasticsearchClusterEnabled :exec
UPDATE monitor_elasticsearch_cluster SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockLogRetentionTierEnabled :one
SELECT enabled FROM monitor_log_retention_tier WHERE id = $1 FOR UPDATE;
-- name: UpdateLogRetentionTierEnabled :exec
UPDATE monitor_log_retention_tier SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockLogCollectionFilterRuleEnabled :one
SELECT enabled FROM monitor_log_collection_filter_rule WHERE id = $1 FOR UPDATE;
-- name: UpdateLogCollectionFilterRuleEnabled :exec
UPDATE monitor_log_collection_filter_rule SET enabled = $1, update_time = $2 WHERE id = $3;

-- name: LockSoftwarePackageEnabled :one
SELECT enabled FROM monitor_software_package WHERE id = $1 FOR UPDATE;
-- name: UpdateSoftwarePackageEnabled :exec
UPDATE monitor_software_package SET enabled = $1, update_time = $2 WHERE id = $3;
