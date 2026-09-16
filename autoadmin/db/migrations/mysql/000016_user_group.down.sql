-- 回滚 000016：删除用户组表与菜单（成员关系随 CASCADE 一并删除）。
DROP TABLE IF EXISTS `sys_user_group_member`;
DROP TABLE IF EXISTS `sys_user_group`;

DELETE rm FROM sys_role_menu rm JOIN sys_menu btn ON rm.menu_id = btn.id WHERE btn.menu_type = 'F' AND btn.perms LIKE 'system:usergroups:%';
DELETE rm FROM sys_role_menu rm JOIN sys_menu ug ON rm.menu_id = ug.id WHERE ug.name = '用户组' AND ug.menu_type = 'C';
DELETE FROM sys_menu WHERE perms LIKE 'system:usergroups:%' AND menu_type = 'F';
DELETE FROM sys_menu WHERE name = '用户组' AND menu_type = 'C';
