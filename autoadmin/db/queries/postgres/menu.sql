-- 本文件由 make derive 从 db/queries/mysql/menu.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: GetMenuByID :one
SELECT * FROM sys_menu WHERE id = $1 LIMIT 1;

-- name: ListMenus :many
SELECT * FROM sys_menu ORDER BY location, order_num, id;

-- name: ListMenusByUserID :many
SELECT DISTINCT m.*
FROM sys_menu AS m
JOIN sys_role_menu AS rm ON rm.menu_id = m.id
JOIN sys_user_role AS ur ON ur.role_id = rm.role_id
WHERE ur.user_id = $1
ORDER BY m.location, m.order_num, m.id;

-- name: ListMenuIDsByRoleID :many
SELECT menu_id FROM sys_role_menu WHERE role_id = $1 ORDER BY menu_id;

-- name: CreateMenu :one
INSERT INTO sys_menu (
  name, icon, parent_id, order_num, path, component, menu_type,
  perms, create_time, update_time, remark, location, is_expanded
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id;

-- name: UpdateMenu :exec
UPDATE sys_menu SET
  name = $1, icon = $2, parent_id = $3, order_num = $4, path = $5, component = $6,
  menu_type = $7, perms = $8, update_time = $9, remark = $10, location = $11, is_expanded = $12
WHERE id = $13;

-- name: DeleteMenuByID :exec
DELETE FROM sys_menu WHERE id = $1;

-- name: DeleteMenuRoles :exec
DELETE FROM sys_role_menu WHERE menu_id = $1;

-- name: AddRoleMenu :exec
INSERT INTO sys_role_menu (role_id, menu_id) VALUES ($1, $2);