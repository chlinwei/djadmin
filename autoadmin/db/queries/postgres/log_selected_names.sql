-- 本文件由 make derive 从 db/queries/mysql/log_selected_names.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: ListSelectedLogTierNames :many
SELECT id, name FROM monitor_log_retention_tier WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ListSelectedLogFilterNames :many
SELECT id, name FROM monitor_log_collection_filter_rule WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ListServiceDefaultTierNames :many
SELECT DISTINCT tier.id, tier.name FROM monitor_log_retention_tier tier
JOIN assets_application_service s ON s.log_retention_tier_id = tier.id
WHERE s.id = ANY(sqlc.arg(ids)::bigint[]);
