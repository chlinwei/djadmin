-- 本文件由 make derive 从 db/queries/mysql/user.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: GetUserByID :one
SELECT * FROM sys_user WHERE id = $1 LIMIT 1;

-- name: GetUserByUsername :one
SELECT * FROM sys_user WHERE username = $1 LIMIT 1;

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
LIMIT $1 OFFSET $2;

-- name: SearchUsers :many
SELECT * FROM sys_user
WHERE username LIKE sqlc.arg(username_pattern)
  OR phonenumber LIKE sqlc.arg(phonenumber_pattern)
  OR remark LIKE sqlc.arg(remark_pattern)
ORDER BY username, id
LIMIT $1 OFFSET $2;

-- name: CreateUser :one
INSERT INTO sys_user (
  username, password, avatar, phonenumber, login_date, status,
  create_time, update_time, remark, timezone
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id;

-- name: UpdateUserProfile :exec
UPDATE sys_user
SET avatar = $1, phonenumber = $2, status = $3, remark = $4, timezone = $5, update_time = $6
WHERE id = $7;

-- name: UpdateUserPassword :exec
UPDATE sys_user SET password = $1, update_time = $2 WHERE id = $3;

-- name: UpdateUserLoginDate :exec
UPDATE sys_user SET login_date = $1 WHERE id = $2;

-- name: DeleteUserByID :exec
DELETE FROM sys_user WHERE id = $1;

-- name: DeleteUserRoles :exec
DELETE FROM sys_user_role WHERE user_id = $1;

-- name: AddUserRole :exec
INSERT INTO sys_user_role (user_id, role_id) VALUES ($1, $2);

-- name: ListRolesByUserID :many
SELECT r.*
FROM sys_role AS r
JOIN sys_user_role AS ur ON ur.role_id = r.id
WHERE ur.user_id = $1
ORDER BY r.name, r.id;

-- name: ListRoleCodesByUserID :many
SELECT r.code
FROM sys_role AS r
JOIN sys_user_role AS ur ON ur.role_id = r.id
WHERE ur.user_id = $1 AND r.code IS NOT NULL
ORDER BY r.code;

-- name: ListPermissionCodesByUserID :many
SELECT DISTINCT m.perms
FROM sys_menu AS m
JOIN sys_role_menu AS rm ON rm.menu_id = m.id
JOIN sys_user_role AS ur ON ur.role_id = rm.role_id
WHERE ur.user_id = $1 AND m.perms IS NOT NULL AND m.perms <> ''
ORDER BY m.perms;

-- name: CreateLoginAudit :exec
INSERT INTO audit_login_log (
  username, user_id, status, client_ip, user_agent, message, login_time
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListAPITokens :many
SELECT t.id, t.agent_id, t.bind_mode, t.name, t.is_active, t.expires_at,
       t.last_used_at, t.created_by_id, u.username AS created_by_username,
       t.remark, t.create_time, t.update_time
FROM sys_agent_token AS t
LEFT JOIN sys_user AS u ON u.id = t.created_by_id
ORDER BY t.id DESC;

-- name: GetAPITokenByID :one
SELECT * FROM sys_agent_token WHERE id = $1 LIMIT 1;

-- name: CountAPITokensByAgentID :one
SELECT COUNT(*) FROM sys_agent_token WHERE bind_mode = 'api' AND agent_id = $1;

-- name: CreateAPIToken :one
INSERT INTO sys_agent_token (
  agent_id, token_hash, name, is_active, expires_at, last_used_at,
  remark, create_time, update_time, created_by_id, bind_mode
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: RotateAPIToken :exec
UPDATE sys_agent_token
SET token_hash = $1, last_used_at = NULL, is_active = TRUE, update_time = $2
WHERE id = $3;

-- name: DisableAPIToken :exec
UPDATE sys_agent_token SET is_active = FALSE, update_time = $1 WHERE id = $2;

-- name: DeleteAPIToken :exec
DELETE FROM sys_agent_token WHERE id = $1;
-- name: UpdateUserPhonenumber :exec
UPDATE sys_user SET phonenumber = $1, update_time = $2 WHERE id = $3;

-- ---- 用户组（sys_user_group / sys_user_group_member，identity 域）----

-- name: ListUserGroups :many
SELECT id, name, COALESCE(remark, '') AS remark, create_time, update_time
FROM sys_user_group ORDER BY id;

-- 一次取回全部成员，调用方按 group_id 归组（原实现就是这么做的，避免 N+1）。
-- name: ListUserGroupMembers :many
SELECT m.group_id, m.user_id, u.username
FROM sys_user_group_member m JOIN sys_user u ON u.id = m.user_id
ORDER BY m.group_id, m.id;

-- name: CreateUserGroup :one
INSERT INTO sys_user_group(create_time, update_time, remark, name) VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: UpdateUserGroup :execrows
UPDATE sys_user_group SET update_time = $1, remark = $2, name = $3 WHERE id = $4;

-- name: DeleteUserGroup :execrows
DELETE FROM sys_user_group WHERE id = $1;

-- name: DeleteUserGroupMembers :exec
DELETE FROM sys_user_group_member WHERE group_id = $1;

-- name: CreateUserGroupMember :exec
INSERT INTO sys_user_group_member(create_time, group_id, user_id) VALUES ($1, $2, $3);

-- ---- 用户中心的告警媒介绑定（monitor_alert_media / monitor_user_alert_media_binding）----
-- 查询定义按「调用方所属域」放这里（同 inspection.sql 里的 assets_* 查询）。

-- name: ListEnabledAlertMedia :many
SELECT id, name, media_type, enabled FROM monitor_alert_media WHERE enabled = TRUE ORDER BY id;

-- 用户中心的"我的告警媒介绑定"与 monitor 的通知链路诊断（user-chain）共用一条：
-- 后两者要的 media_type / media_enabled 补在列尾（身份域只按字段名取前几列，不受影响）。
-- name: ListUserAlertMediaBindings :many
SELECT b.id, b.media_id, m.name AS media_name, b.recipients, b.enabled,
       m.media_type, m.enabled AS media_enabled
FROM monitor_user_alert_media_binding b JOIN monitor_alert_media m ON m.id = b.media_id
WHERE b.user_id = $1 ORDER BY b.id;

-- name: DeleteUserAlertMediaBindings :exec
DELETE FROM monitor_user_alert_media_binding WHERE user_id = $1;

-- name: CreateUserAlertMediaBinding :exec
INSERT INTO monitor_user_alert_media_binding(create_time, update_time, remark, recipients, enabled, media_id, user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7);
