-- 回滚监控中心菜单整合：子菜单 path 改回旧扁平值并直接挂回监控中心；删除三个二级目录。
-- 目录按 path 定位删除，子菜单按旧 path 条件恢复（幂等）。

UPDATE sys_menu m JOIN sys_menu p ON p.path = '/monitor' AND p.menu_type = 'M'
SET m.parent_id = p.id, m.path = '/monitor', m.order_num = 1, m.update_time = CURDATE()
WHERE m.path = '/monitor/alerting/dashboard';
UPDATE sys_menu m JOIN sys_menu p ON p.path = '/monitor' AND p.menu_type = 'M'
SET m.parent_id = p.id, m.path = '/monitor/alert-rules', m.order_num = 2, m.update_time = CURDATE()
WHERE m.path = '/monitor/alerting/rules';
UPDATE sys_menu m JOIN sys_menu p ON p.path = '/monitor' AND p.menu_type = 'M'
SET m.parent_id = p.id, m.path = '/monitor/alerts', m.order_num = 3, m.update_time = CURDATE()
WHERE m.path = '/monitor/alerting/alerts';
UPDATE sys_menu m JOIN sys_menu p ON p.path = '/monitor' AND p.menu_type = 'M'
SET m.parent_id = p.id, m.path = '/monitor/explore', m.order_num = 4, m.update_time = CURDATE()
WHERE m.path = '/monitor/alerting/explore';

UPDATE sys_menu m JOIN sys_menu p ON p.path = '/monitor' AND p.menu_type = 'M'
SET m.parent_id = p.id, m.path = '/monitor/media', m.order_num = 5, m.update_time = CURDATE()
WHERE m.path = '/monitor/notification/media';
UPDATE sys_menu m JOIN sys_menu p ON p.path = '/monitor' AND p.menu_type = 'M'
SET m.parent_id = p.id, m.path = '/monitor/notification-policies', m.order_num = 6, m.update_time = CURDATE()
WHERE m.path = '/monitor/notification/policies';

UPDATE sys_menu m JOIN sys_menu p ON p.path = '/monitor' AND p.menu_type = 'M'
SET m.parent_id = p.id, m.path = '/monitor/log-storage', m.order_num = 20, m.update_time = CURDATE()
WHERE m.path = '/monitor/logging/storage';
UPDATE sys_menu m JOIN sys_menu p ON p.path = '/monitor' AND p.menu_type = 'M'
SET m.parent_id = p.id, m.path = '/monitor/log-parsers', m.order_num = 21, m.update_time = CURDATE()
WHERE m.path = '/monitor/logging/parsers';
UPDATE sys_menu m JOIN sys_menu p ON p.path = '/monitor' AND p.menu_type = 'M'
SET m.parent_id = p.id, m.path = '/monitor/log-retention', m.order_num = 22, m.update_time = CURDATE()
WHERE m.path = '/monitor/logging/retention';

DELETE FROM sys_role_menu
WHERE menu_id IN (SELECT id FROM (SELECT id FROM sys_menu WHERE path IN ('/monitor/alerting', '/monitor/notification', '/monitor/logging') AND menu_type = 'M') t);

DELETE FROM sys_menu WHERE path IN ('/monitor/alerting', '/monitor/notification', '/monitor/logging') AND menu_type = 'M';
