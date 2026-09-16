-- name: GetConfigByID :one
SELECT * FROM sys_config WHERE id = ? LIMIT 1;

-- name: GetConfigByKey :one
SELECT * FROM sys_config WHERE `key` = ? LIMIT 1;

-- name: CountConfigs :one
SELECT COUNT(*) FROM sys_config;

-- name: CountConfigsBySearch :one
SELECT COUNT(*) FROM sys_config
WHERE name LIKE sqlc.arg(name_pattern)
  OR `key` LIKE sqlc.arg(key_pattern);

-- name: ListConfigs :many
SELECT * FROM sys_config
ORDER BY id
LIMIT ? OFFSET ?;

-- name: SearchConfigs :many
SELECT * FROM sys_config
WHERE name LIKE sqlc.arg(name_pattern)
  OR `key` LIKE sqlc.arg(key_pattern)
ORDER BY id
LIMIT ? OFFSET ?;

-- name: CreateConfig :execresult
INSERT INTO sys_config (
  create_time, update_time, remark, `key`, value, value_type,
  name, description, is_readonly, default_value
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateConfigValue :exec
UPDATE sys_config SET value = ?, default_value = ?, update_time = ? WHERE id = ?;

-- name: ResetConfigValue :exec
UPDATE sys_config SET value = default_value, update_time = ? WHERE id = ?;