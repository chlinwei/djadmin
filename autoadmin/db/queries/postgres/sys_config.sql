-- 本文件由 make derive 从 db/queries/mysql/sys_config.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: GetConfigByID :one
SELECT * FROM sys_config WHERE id = $1 LIMIT 1;

-- name: GetConfigByKey :one
SELECT * FROM sys_config WHERE "key" = $1 LIMIT 1;

-- name: CountConfigs :one
SELECT COUNT(*) FROM sys_config;

-- name: CountConfigsBySearch :one
SELECT COUNT(*) FROM sys_config
WHERE name LIKE sqlc.arg(name_pattern)
  OR "key" LIKE sqlc.arg(key_pattern);

-- name: ListConfigs :many
SELECT * FROM sys_config
ORDER BY id
LIMIT $1 OFFSET $2;

-- name: SearchConfigs :many
SELECT * FROM sys_config
WHERE name LIKE sqlc.arg(name_pattern)
  OR "key" LIKE sqlc.arg(key_pattern)
ORDER BY id
LIMIT $1 OFFSET $2;

-- name: CreateConfig :one
INSERT INTO sys_config (
  create_time, update_time, remark, "key", value, value_type,
  name, description, is_readonly, default_value
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id;

-- name: UpdateConfigValue :exec
UPDATE sys_config SET value = $1, default_value = $2, update_time = $3 WHERE id = $4;

-- name: ResetConfigValue :exec
UPDATE sys_config SET value = default_value, update_time = $1 WHERE id = $2;