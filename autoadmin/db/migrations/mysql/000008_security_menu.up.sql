-- 安全中心：顶层菜单（M，与「自动化运维」「监控中心」平级）+ 基线扫描子菜单（C）与按钮权限（F）。
-- 注意：sys_menu 沿用 Django 风格枚举 M/C/F；location=1、is_expanded 与现有菜单保持一致。
INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '安全中心', 'fa-shield-halved', 0, 70, '/security', NULL, 'M', NULL, 1, TRUE, NOW(6), NOW(6)
WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE name = '安全中心');

SET @security_parent := (SELECT id FROM sys_menu WHERE name = '安全中心' LIMIT 1);

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '基线扫描', 'fa-lock', @security_parent, 1, '/sys/security/baseline', 'security/baseline/index', 'C', 'baseline:manage', 1, TRUE, NOW(6), NOW(6)
WHERE @security_parent IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE name = '基线扫描');

SET @baseline_page := (SELECT id FROM sys_menu WHERE name = '基线扫描' LIMIT 1);

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '基线管理', NULL, @baseline_page, 1, '', NULL, 'F', 'baseline:manage', 1, FALSE, NOW(6), NOW(6)
WHERE @baseline_page IS NOT NULL;

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, location, is_expanded, create_time, update_time)
SELECT '发起扫描', NULL, @baseline_page, 2, '', NULL, 'F', 'baseline:scan', 1, FALSE, NOW(6), NOW(6)
WHERE @baseline_page IS NOT NULL;

-- 角色授权：安全中心为顶层菜单，授权给所有已拥有「自动化运维」菜单的角色（维持原可见人群）。
-- 安全中心 → 基线扫描 → 按钮权限，逐级继承。
INSERT IGNORE INTO sys_role_menu (role_id, menu_id)
SELECT DISTINCT rm.role_id, sec.id
FROM sys_menu automation
JOIN sys_role_menu rm ON rm.menu_id = automation.id
JOIN sys_menu sec ON sec.parent_id = 0 AND sec.name = '安全中心'
WHERE automation.name = '自动化运维';

INSERT IGNORE INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, child.id
FROM sys_menu sec
JOIN sys_role_menu rm ON rm.menu_id = sec.id
JOIN sys_menu child ON child.parent_id = sec.id AND child.name = '基线扫描'
WHERE sec.name = '安全中心';

INSERT IGNORE INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, btn.id
FROM sys_menu baseline_menu
JOIN sys_role_menu rm ON rm.menu_id = baseline_menu.id
JOIN sys_menu btn ON btn.parent_id = baseline_menu.id AND btn.menu_type = 'F'
WHERE baseline_menu.name = '基线扫描';
