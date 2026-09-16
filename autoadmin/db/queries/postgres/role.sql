-- 本文件由 make derive 从 db/queries/mysql/role.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: GetRoleByID :one
SELECT * FROM sys_role WHERE id = $1 LIMIT 1;

-- name: CountRoles :one
SELECT COUNT(*) FROM sys_role;

-- name: CountRolesBySearch :one
SELECT COUNT(*) FROM sys_role
WHERE name LIKE sqlc.arg(name_pattern)
  OR code LIKE sqlc.arg(code_pattern)
  OR remark LIKE sqlc.arg(remark_pattern);

-- name: ListRoles :many
SELECT * FROM sys_role
ORDER BY name, id
LIMIT $1 OFFSET $2;

-- name: SearchRoles :many
SELECT * FROM sys_role
WHERE name LIKE sqlc.arg(name_pattern)
  OR code LIKE sqlc.arg(code_pattern)
  OR remark LIKE sqlc.arg(remark_pattern)
ORDER BY name, id
LIMIT $1 OFFSET $2;

-- name: CreateRole :one
INSERT INTO sys_role (name, code, create_time, update_time, remark)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: UpdateRole :exec
UPDATE sys_role SET name = $1, code = $2, update_time = $3, remark = $4 WHERE id = $5;

-- name: DeleteRoleByID :exec
DELETE FROM sys_role WHERE id = $1;

-- name: DeleteRoleMenus :exec
DELETE FROM sys_role_menu WHERE role_id = $1;

-- name: DeleteRoleUsers :exec
DELETE FROM sys_user_role WHERE role_id = $1;