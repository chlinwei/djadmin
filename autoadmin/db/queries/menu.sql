-- name: GetMenuByID :one
SELECT * FROM sys_menu WHERE id = ? LIMIT 1;

-- name: ListMenus :many
SELECT * FROM sys_menu ORDER BY location, order_num, id;

-- name: ListMenusByUserID :many
SELECT DISTINCT m.*
FROM sys_menu AS m
JOIN sys_role_menu AS rm ON rm.menu_id = m.id
JOIN sys_user_role AS ur ON ur.role_id = rm.role_id
WHERE ur.user_id = ?
ORDER BY m.location, m.order_num, m.id;

-- name: ListMenuIDsByRoleID :many
SELECT menu_id FROM sys_role_menu WHERE role_id = ? ORDER BY menu_id;

-- name: CreateMenu :execresult
INSERT INTO sys_menu (
  name, icon, parent_id, order_num, path, component, menu_type,
  perms, create_time, update_time, remark, location, is_expanded
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateMenu :exec
UPDATE sys_menu SET
  name = ?, icon = ?, parent_id = ?, order_num = ?, path = ?, component = ?,
  menu_type = ?, perms = ?, update_time = ?, remark = ?, location = ?, is_expanded = ?
WHERE id = ?;

-- name: DeleteMenuByID :exec
DELETE FROM sys_menu WHERE id = ?;

-- name: DeleteMenuRoles :exec
DELETE FROM sys_role_menu WHERE menu_id = ?;

-- name: AddRoleMenu :exec
INSERT INTO sys_role_menu (role_id, menu_id) VALUES (?, ?);