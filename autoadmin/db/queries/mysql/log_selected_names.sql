-- name: ListSelectedLogTierNames :many
SELECT id, name FROM monitor_log_retention_tier WHERE id IN (sqlc.slice(ids));

-- name: ListSelectedLogFilterNames :many
SELECT id, name FROM monitor_log_collection_filter_rule WHERE id IN (sqlc.slice(ids));

-- name: ListServiceDefaultTierNames :many
SELECT DISTINCT tier.id, tier.name FROM monitor_log_retention_tier tier
JOIN assets_application_service s ON s.log_retention_tier_id = tier.id
WHERE s.id IN (sqlc.slice(ids));
