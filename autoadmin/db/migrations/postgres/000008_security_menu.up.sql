-- 安全中心：顶层菜单（M，与「自动化运维」「监控中心」平级）+ 基线扫描子菜单（C）与按钮权限（F）。
-- 注意：sys_menu 沿用 Django 风格枚举 M/C/F；location=1、is_expanded 与现有菜单保持一致。
--
-- PG 侧差异：
--   1) PG 没有 MySQL 的会话变量 SET @x := (...)，改为在 INSERT ... SELECT 里用
--      子查询 / 自连接 sys_menu（按 name 定位父节点）取父 id，判断条件语义等价；
--   2) sys_menu.create_time/update_time 在 PG 是 date，now() 由 PG 隐式转成当天日期
--      （与 MySQL 落库同为当天，列类型本身就会截断时间部分）；
--   3) INSERT IGNORE → ON CONFLICT DO NOTHING（sys_role_menu 的唯一键是 (menu_id, role_id)）。

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '安全中心', 'fa-shield-halved', 0, 70, '/security', NULL, 'M', NULL, 1, TRUE, now(), now()
WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE name = '安全中心');

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '基线扫描', 'fa-lock', p.id, 1, '/sys/security/baseline', 'security/baseline/index', 'C', 'baseline:manage', 1, TRUE, now(), now()
FROM sys_menu p
WHERE p.name = '安全中心'
  AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE name = '基线扫描');

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '基线管理', NULL, p.id, 1, '', NULL, 'F', 'baseline:manage', 1, FALSE, now(), now()
FROM sys_menu p
WHERE p.name = '基线扫描';

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '发起扫描', NULL, p.id, 2, '', NULL, 'F', 'baseline:scan', 1, FALSE, now(), now()
FROM sys_menu p
WHERE p.name = '基线扫描';

-- 角色授权：安全中心为顶层菜单，授权给所有已拥有「自动化运维」菜单的角色（维持原可见人群）。
-- 安全中心 → 基线扫描 → 按钮权限，逐级继承。
INSERT INTO sys_role_menu (role_id, menu_id)
SELECT DISTINCT rm.role_id, sec.id
FROM sys_menu automation
JOIN sys_role_menu rm ON rm.menu_id = automation.id
JOIN sys_menu sec ON sec.parent_id = 0 AND sec.name = '安全中心'
WHERE automation.name = '自动化运维'
ON CONFLICT DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, child.id
FROM sys_menu sec
JOIN sys_role_menu rm ON rm.menu_id = sec.id
JOIN sys_menu child ON child.parent_id = sec.id AND child.name = '基线扫描'
WHERE sec.name = '安全中心'
ON CONFLICT DO NOTHING;

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, btn.id
FROM sys_menu baseline_menu
JOIN sys_role_menu rm ON rm.menu_id = baseline_menu.id
JOIN sys_menu btn ON btn.parent_id = baseline_menu.id AND btn.menu_type = 'F'
WHERE baseline_menu.name = '基线扫描'
ON CONFLICT DO NOTHING;
