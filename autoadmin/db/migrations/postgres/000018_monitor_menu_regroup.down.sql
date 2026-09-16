-- 回滚监控中心菜单整合：子菜单 path 改回旧扁平值并直接挂回监控中心；删除三个二级目录。
-- 目录按 path 定位删除，子菜单按旧 path 条件恢复（幂等）。
--
-- PG 侧差异：同 000018 up——多表 UPDATE ... JOIN → UPDATE ... FROM；CURDATE() → CURRENT_DATE。

UPDATE sys_menu m
SET parent_id = p.id, path = '/monitor', order_num = 1, update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor' AND p.menu_type = 'M'
  AND m.path = '/monitor/alerting/dashboard';
UPDATE sys_menu m
SET parent_id = p.id, path = '/monitor/alert-rules', order_num = 2, update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor' AND p.menu_type = 'M'
  AND m.path = '/monitor/alerting/rules';
UPDATE sys_menu m
SET parent_id = p.id, path = '/monitor/alerts', order_num = 3, update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor' AND p.menu_type = 'M'
  AND m.path = '/monitor/alerting/alerts';
UPDATE sys_menu m
SET parent_id = p.id, path = '/monitor/explore', order_num = 4, update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor' AND p.menu_type = 'M'
  AND m.path = '/monitor/alerting/explore';

UPDATE sys_menu m
SET parent_id = p.id, path = '/monitor/media', order_num = 5, update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor' AND p.menu_type = 'M'
  AND m.path = '/monitor/notification/media';
UPDATE sys_menu m
SET parent_id = p.id, path = '/monitor/notification-policies', order_num = 6, update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor' AND p.menu_type = 'M'
  AND m.path = '/monitor/notification/policies';

UPDATE sys_menu m
SET parent_id = p.id, path = '/monitor/log-storage', order_num = 20, update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor' AND p.menu_type = 'M'
  AND m.path = '/monitor/logging/storage';
UPDATE sys_menu m
SET parent_id = p.id, path = '/monitor/log-parsers', order_num = 21, update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor' AND p.menu_type = 'M'
  AND m.path = '/monitor/logging/parsers';
UPDATE sys_menu m
SET parent_id = p.id, path = '/monitor/log-retention', order_num = 22, update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor' AND p.menu_type = 'M'
  AND m.path = '/monitor/logging/retention';

DELETE FROM sys_role_menu
WHERE menu_id IN (SELECT id FROM (SELECT id FROM sys_menu WHERE path IN ('/monitor/alerting', '/monitor/notification', '/monitor/logging') AND menu_type = 'M') t);

DELETE FROM sys_menu WHERE path IN ('/monitor/alerting', '/monitor/notification', '/monitor/logging') AND menu_type = 'M';
