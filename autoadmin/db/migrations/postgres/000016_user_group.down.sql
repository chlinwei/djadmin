-- 回滚 000016：删除用户组表与菜单（成员关系随 CASCADE 一并删除）。
-- PG 侧差异：MySQL 的多表 DELETE（DELETE rm FROM sys_role_menu rm JOIN sys_menu ...）
--   改写为 DELETE ... WHERE menu_id IN (子查询)，语义等价。
DROP TABLE IF EXISTS sys_user_group_member;
DROP TABLE IF EXISTS sys_user_group;

DELETE FROM sys_role_menu WHERE menu_id IN (
  SELECT id FROM sys_menu WHERE menu_type = 'F' AND perms LIKE 'system:usergroups:%'
);
DELETE FROM sys_role_menu WHERE menu_id IN (
  SELECT id FROM sys_menu WHERE name = '用户组' AND menu_type = 'C'
);
DELETE FROM sys_menu WHERE perms LIKE 'system:usergroups:%' AND menu_type = 'F';
DELETE FROM sys_menu WHERE name = '用户组' AND menu_type = 'C';
