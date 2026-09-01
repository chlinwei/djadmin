-- name: GetUserByID :one
SELECT * FROM sys_user WHERE id = ? LIMIT 1;

-- name: GetUserByUsername :one
SELECT * FROM sys_user WHERE username = ? LIMIT 1;

-- name: CountUsers :one
SELECT COUNT(*) FROM sys_user;

-- name: CountUsersBySearch :one
SELECT COUNT(*) FROM sys_user
WHERE username LIKE sqlc.arg(username_pattern)
  OR phonenumber LIKE sqlc.arg(phonenumber_pattern)
  OR remark LIKE sqlc.arg(remark_pattern);

-- name: ListUsers :many
SELECT * FROM sys_user
ORDER BY username, id
LIMIT ? OFFSET ?;

-- name: SearchUsers :many
SELECT * FROM sys_user
WHERE username LIKE sqlc.arg(username_pattern)
  OR phonenumber LIKE sqlc.arg(phonenumber_pattern)
  OR remark LIKE sqlc.arg(remark_pattern)
ORDER BY username, id
LIMIT ? OFFSET ?;

-- name: CreateUser :execresult
INSERT INTO sys_user (
  username, password, avatar, phonenumber, login_date, status,
  create_time, update_time, remark, timezone
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateUserProfile :exec
UPDATE sys_user
SET avatar = ?, phonenumber = ?, status = ?, remark = ?, timezone = ?, update_time = ?
WHERE id = ?;

-- name: UpdateUserPassword :exec
UPDATE sys_user SET password = ?, update_time = ? WHERE id = ?;

-- name: UpdateUserLoginDate :exec
UPDATE sys_user SET login_date = ? WHERE id = ?;

-- name: DeleteUserByID :exec
DELETE FROM sys_user WHERE id = ?;

-- name: DeleteUserRoles :exec
DELETE FROM sys_user_role WHERE user_id = ?;

-- name: AddUserRole :exec
INSERT INTO sys_user_role (user_id, role_id) VALUES (?, ?);

-- name: ListRolesByUserID :many
SELECT r.*
FROM sys_role AS r
JOIN sys_user_role AS ur ON ur.role_id = r.id
WHERE ur.user_id = ?
ORDER BY r.name, r.id;

-- name: ListRoleCodesByUserID :many
SELECT r.code
FROM sys_role AS r
JOIN sys_user_role AS ur ON ur.role_id = r.id
WHERE ur.user_id = ? AND r.code IS NOT NULL
ORDER BY r.code;

-- name: ListPermissionCodesByUserID :many
SELECT DISTINCT m.perms
FROM sys_menu AS m
JOIN sys_role_menu AS rm ON rm.menu_id = m.id
JOIN sys_user_role AS ur ON ur.role_id = rm.role_id
WHERE ur.user_id = ? AND m.perms IS NOT NULL AND m.perms <> ''
ORDER BY m.perms;

-- name: CreateLoginAudit :exec
INSERT INTO audit_login_log (
  username, user_id, status, client_ip, user_agent, message, login_time
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListAPITokens :many
SELECT t.id, t.agent_id, t.bind_mode, t.name, t.is_active, t.expires_at,
       t.last_used_at, t.created_by_id, u.username AS created_by_username,
       t.remark, t.create_time, t.update_time
FROM sys_agent_token AS t
LEFT JOIN sys_user AS u ON u.id = t.created_by_id
ORDER BY t.id DESC;

-- name: GetAPITokenByID :one
SELECT * FROM sys_agent_token WHERE id = ? LIMIT 1;

-- name: CountAPITokensByAgentID :one
SELECT COUNT(*) FROM sys_agent_token WHERE bind_mode = 'api' AND agent_id = ?;

-- name: CreateAPIToken :execresult
INSERT INTO sys_agent_token (
  agent_id, token_hash, name, is_active, expires_at, last_used_at,
  remark, create_time, update_time, created_by_id, bind_mode
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: RotateAPIToken :exec
UPDATE sys_agent_token
SET token_hash = ?, last_used_at = NULL, is_active = TRUE, update_time = ?
WHERE id = ?;

-- name: DisableAPIToken :exec
UPDATE sys_agent_token SET is_active = FALSE, update_time = ? WHERE id = ?;

-- name: DeleteAPIToken :exec
DELETE FROM sys_agent_token WHERE id = ?;