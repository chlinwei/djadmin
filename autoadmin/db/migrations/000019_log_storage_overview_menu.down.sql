-- 回滚「存储水位」菜单：先清授权再删菜单（按 path 定位，幂等）。
DELETE FROM sys_role_menu WHERE menu_id IN (
    SELECT id FROM (SELECT id FROM sys_menu WHERE path = '/monitor/logging/overview') t
);
DELETE FROM sys_menu WHERE path = '/monitor/logging/overview';
