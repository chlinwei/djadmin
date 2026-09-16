-- 监控中心菜单整合：9 个平铺菜单按职能挂入 3 个二级目录（监控告警/通知管理/日志管理），
-- 菜单 path 同步改为嵌套结构。SQL 全部幂等（按 path/名称条件判断），对已手动整理过的环境重复执行无害。
-- 旧 path 由前端路由 redirect 兜底（见 fronted/src/router/index.js），不在此保留旧菜单行。
--
-- PG 侧差异：
--   1) MySQL 的多表 UPDATE ... JOIN ... SET（用目录行的 id 改子菜单的 parent_id）→ PG 的
--      UPDATE ... FROM 自连接（PG 的 SET 目标列不能带别名，所以列名与 WHERE 都在子表别名 m 上）；
--   2) CURDATE() → CURRENT_DATE；
--   3) is_expanded 的整型 0 → FALSE（PG 是 boolean，不接受整数隐式转换）；
--   4) INSERT IGNORE → ON CONFLICT DO NOTHING（sys_role_menu 唯一键 (menu_id, role_id)）。

-- 1. 三个二级目录（menu_type=M，挂在监控中心 104 下；parent_id 用 path 定位，避免依赖自增 id）
INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, create_time, update_time, remark, location, is_expanded)
SELECT t.name, t.icon, p.id, t.order_num, t.path, '', 'M', '', CURRENT_DATE, CURRENT_DATE, t.name, p.location, FALSE
FROM (SELECT '监控告警' AS name, 'bell' AS icon, '/monitor/alerting' AS path, 1 AS order_num UNION ALL
      SELECT '通知管理', 'paper-plane', '/monitor/notification', 2 UNION ALL
      SELECT '日志管理', 'file-alt', '/monitor/logging', 3) t
JOIN sys_menu p ON p.path = '/monitor' AND p.menu_type = 'M'
WHERE NOT EXISTS (SELECT 1 FROM sys_menu m WHERE m.path = t.path AND m.parent_id = p.id);

-- 2. 子菜单挂入目录并改嵌套 path（按旧 path 定位，幂等：已迁移的行不再匹配旧 path）
UPDATE sys_menu m
SET parent_id = p.id, order_num = 1, path = '/monitor/alerting/dashboard', update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor/alerting' AND p.menu_type = 'M'
  AND m.path = '/monitor' AND m.menu_type = 'C' AND m.name = '智能监控';
UPDATE sys_menu m
SET parent_id = p.id, order_num = 2, path = '/monitor/alerting/rules', update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor/alerting' AND p.menu_type = 'M'
  AND m.path = '/monitor/alert-rules';
UPDATE sys_menu m
SET parent_id = p.id, order_num = 3, path = '/monitor/alerting/alerts', update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor/alerting' AND p.menu_type = 'M'
  AND m.path = '/monitor/alerts';
UPDATE sys_menu m
SET parent_id = p.id, order_num = 4, path = '/monitor/alerting/explore', update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor/alerting' AND p.menu_type = 'M'
  AND m.path = '/monitor/explore';

UPDATE sys_menu m
SET parent_id = p.id, order_num = 1, path = '/monitor/notification/media', update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor/notification' AND p.menu_type = 'M'
  AND m.path = '/monitor/media';
UPDATE sys_menu m
SET parent_id = p.id, order_num = 2, path = '/monitor/notification/policies', update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor/notification' AND p.menu_type = 'M'
  AND m.path = '/monitor/notification-policies';

UPDATE sys_menu m
SET parent_id = p.id, order_num = 1, path = '/monitor/logging/storage', update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor/logging' AND p.menu_type = 'M'
  AND m.path = '/monitor/log-storage';
UPDATE sys_menu m
SET parent_id = p.id, order_num = 2, path = '/monitor/logging/parsers', update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor/logging' AND p.menu_type = 'M'
  AND m.path = '/monitor/log-parsers';
UPDATE sys_menu m
SET parent_id = p.id, order_num = 3, path = '/monitor/logging/retention', update_time = CURRENT_DATE
FROM sys_menu p
WHERE p.path = '/monitor/logging' AND p.menu_type = 'M'
  AND m.path = '/monitor/log-retention';

-- 3. 目录是权限边界：给所有拥有监控中心的角色补授三个新目录（sys_role_menu 按菜单 id 授权，
-- 不补授则前端菜单树裁掉目录、子菜单全部不可见）。INSERT IGNORE 保证幂等。
INSERT INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, d.id FROM sys_role_menu rm
JOIN sys_menu d ON d.id IN (
    SELECT id FROM sys_menu WHERE path IN ('/monitor/alerting', '/monitor/notification', '/monitor/logging') AND menu_type = 'M'
)
WHERE rm.menu_id = (
    SELECT id FROM sys_menu WHERE path = '/monitor' AND menu_type = 'M' LIMIT 1
)
ON CONFLICT DO NOTHING;
