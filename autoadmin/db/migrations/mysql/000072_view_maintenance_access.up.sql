-- 目录和页面均按菜单 ID 授权；不选择页面下的动作节点。
INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.code = 'view'
  AND ((m.menu_type = 'M' AND m.path IN ('/monitor', '/monitor/notification') AND COALESCE(m.perms, '') IN ('', 'monitor:view'))
    OR (m.menu_type = 'C' AND m.path = '/monitor/notification/maintenance' AND m.perms = 'monitor:maintenance:view'))
  AND NOT EXISTS (SELECT 1 FROM sys_role_menu g WHERE g.role_id = r.id AND g.menu_id = m.id);

-- 查询还要求 monitor:view；若角色已经从任意菜单取得该权限，不再添加额外页面。
INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.code = 'view' AND m.menu_type = 'C' AND m.component = 'monitor/index' AND m.perms = 'monitor:view'
  AND NOT EXISTS (SELECT 1 FROM sys_role_menu g JOIN sys_menu p ON p.id = g.menu_id
                  WHERE g.role_id = r.id AND p.perms = 'monitor:view');
