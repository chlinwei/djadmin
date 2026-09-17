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
SELECT t.id, t.api_id, t.bind_mode, t.name, t.is_active, t.expires_at,
       t.last_used_at, t.created_by_id, u.username AS created_by_username,
       t.remark, t.create_time, t.update_time
FROM sys_agent_token AS t
LEFT JOIN sys_user AS u ON u.id = t.created_by_id
ORDER BY t.id DESC;

-- name: GetAPITokenByID :one
SELECT * FROM sys_agent_token WHERE id = ? LIMIT 1;

-- name: CountAPITokensByApiID :one
SELECT COUNT(*) FROM sys_agent_token WHERE bind_mode = 'api' AND api_id = ?;

-- name: CreateAPIToken :execresult
INSERT INTO sys_agent_token (
  api_id, token_hash, name, is_active, expires_at, last_used_at,
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
-- name: UpdateUserPhonenumber :exec
UPDATE sys_user SET phonenumber = ?, update_time = ? WHERE id = ?;

-- ---- 用户组（sys_user_group / sys_user_group_member，identity 域）----

-- name: ListUserGroups :many
SELECT id, name, COALESCE(remark, '') AS remark, create_time, update_time
FROM sys_user_group ORDER BY id;

-- 一次取回全部成员，调用方按 group_id 归组（原实现就是这么做的，避免 N+1）。
-- name: ListUserGroupMembers :many
SELECT m.group_id, m.user_id, u.username
FROM sys_user_group_member m JOIN sys_user u ON u.id = m.user_id
ORDER BY m.group_id, m.id;

-- name: CreateUserGroup :execresult
INSERT INTO sys_user_group(create_time, update_time, remark, name) VALUES (?, ?, ?, ?);

-- name: UpdateUserGroup :execrows
UPDATE sys_user_group SET update_time = ?, remark = ?, name = ? WHERE id = ?;

-- name: DeleteUserGroup :execrows
DELETE FROM sys_user_group WHERE id = ?;

-- name: DeleteUserGroupMembers :exec
DELETE FROM sys_user_group_member WHERE group_id = ?;

-- name: CreateUserGroupMember :exec
INSERT INTO sys_user_group_member(create_time, group_id, user_id) VALUES (?, ?, ?);

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
WHERE b.user_id = ? ORDER BY b.id;

-- name: DeleteUserAlertMediaBindings :exec
DELETE FROM monitor_user_alert_media_binding WHERE user_id = ?;

-- name: CreateUserAlertMediaBinding :exec
INSERT INTO monitor_user_alert_media_binding(create_time, update_time, remark, recipients, enabled, media_id, user_id)
VALUES (?, ?, ?, ?, ?, ?, ?);
