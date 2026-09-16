DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE perms IN ('baseline:manage','baseline:scan'));
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE name IN ('基线扫描','安全中心'));
DELETE FROM sys_menu WHERE perms IN ('baseline:manage','baseline:scan');
DELETE FROM sys_menu WHERE name = '基线扫描';
DELETE FROM sys_menu WHERE name = '安全中心';
