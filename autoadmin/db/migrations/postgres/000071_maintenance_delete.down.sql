DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE remark='migration:000071:maintenance');
DELETE FROM sys_menu WHERE remark='migration:000071:maintenance';
