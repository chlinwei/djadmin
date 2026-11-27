-- 按角色编码授权，不按登录用户名授权；仅增加维护窗口的查看页面。
INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.code = 'readonly' AND m.menu_type = 'C'
  AND m.path = '/monitor/notification/maintenance' AND m.perms = 'monitor:maintenance:view'
  AND NOT EXISTS (SELECT 1 FROM sys_role_menu g WHERE g.role_id = r.id AND g.menu_id = m.id);
