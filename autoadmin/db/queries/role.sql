-- name: GetRoleByID :one
SELECT * FROM sys_role WHERE id = ? LIMIT 1;

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
LIMIT ? OFFSET ?;

-- name: SearchRoles :many
SELECT * FROM sys_role
WHERE name LIKE sqlc.arg(name_pattern)
  OR code LIKE sqlc.arg(code_pattern)
  OR remark LIKE sqlc.arg(remark_pattern)
ORDER BY name, id
LIMIT ? OFFSET ?;

-- name: CreateRole :execresult
INSERT INTO sys_role (name, code, create_time, update_time, remark)
VALUES (?, ?, ?, ?, ?);

-- name: UpdateRole :exec
UPDATE sys_role SET name = ?, code = ?, update_time = ?, remark = ? WHERE id = ?;

-- name: DeleteRoleByID :exec
DELETE FROM sys_role WHERE id = ?;

-- name: DeleteRoleMenus :exec
DELETE FROM sys_role_menu WHERE role_id = ?;

-- name: DeleteRoleUsers :exec
DELETE FROM sys_user_role WHERE role_id = ?;