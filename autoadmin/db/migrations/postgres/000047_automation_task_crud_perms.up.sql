-- 自动化任务缺少创建/编辑/删除权限点（2026-09-21 现场：非 admin 角色「新增任务」保存报「无权限访问」）。
-- 任务列表的写接口按 `automation:tasks:{create,update,delete}` 鉴权（router.go），前端按钮也按同码显隐，
-- 但 sys_menu 里从来只有 `automation:tasks:view` 这一条——权限码在 RBAC 里不存在，任何角色都授不到，
-- 所以一律 403。本迁移补三条 F 型权限点，并补授权给已拥有 `automation:tasks:view` 的角色。
--
-- 同时纠正一处历史误命名：旧行「任务创建」的 perms 其实是 `automation:jobs:create`，它控制的是
-- run_now/precheck（作业执行，前端「执行」按钮也用它），与任务 CRUD 是两回事，改名为「任务执行」。
--
-- 注意：权限码随 JWT 签发，存量登录用户需重新登录后新权限才生效。
--
-- PG 侧差异：CURDATE() → CURRENT_DATE；is_expanded 的 1 → TRUE；INSERT IGNORE → ON CONFLICT DO NOTHING。

-- 1) 纠正既有名称（仅当该行仍是 jobs:create 时，避免覆盖人工改名）。
UPDATE sys_menu SET name = '任务执行', update_time = CURRENT_DATE
WHERE name = '任务创建' AND perms = 'automation:jobs:create';

-- 2) 补三条任务 CRUD 权限点，挂在「任务查看」（automation:tasks:view）的同一父节点下。
INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, create_time, update_time, remark, location, is_expanded)
SELECT '任务新增', NULL, parent_id, 8, '', NULL, 'F', 'automation:tasks:create', CURRENT_DATE, CURRENT_DATE, '自动化任务新增', location, TRUE
FROM sys_menu WHERE perms = 'automation:tasks:view'
  AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE perms = 'automation:tasks:create');

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, create_time, update_time, remark, location, is_expanded)
SELECT '任务编辑', NULL, parent_id, 9, '', NULL, 'F', 'automation:tasks:update', CURRENT_DATE, CURRENT_DATE, '自动化任务编辑', location, TRUE
FROM sys_menu WHERE perms = 'automation:tasks:view'
  AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE perms = 'automation:tasks:update');

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, create_time, update_time, remark, location, is_expanded)
SELECT '任务删除', NULL, parent_id, 10, '', NULL, 'F', 'automation:tasks:delete', CURRENT_DATE, CURRENT_DATE, '自动化任务删除', location, TRUE
FROM sys_menu WHERE perms = 'automation:tasks:view'
  AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE perms = 'automation:tasks:delete');

-- 3) 补授权：拥有「任务查看」的角色自动获得三个任务写权限（不补授则菜单可见但操作仍 403）。
INSERT INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, m.id FROM sys_role_menu rm
JOIN sys_menu m ON m.perms IN ('automation:tasks:create', 'automation:tasks:update', 'automation:tasks:delete')
WHERE rm.menu_id = (SELECT id FROM sys_menu WHERE perms = 'automation:tasks:view' LIMIT 1)
ON CONFLICT DO NOTHING;
