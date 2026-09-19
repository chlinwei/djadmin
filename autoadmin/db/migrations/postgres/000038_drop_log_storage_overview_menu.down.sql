-- 还原「存储水位」菜单（000019 的原样重建）：字段与 000019 的 up 逐字一致，
-- 菜单名/路径必须与 MySQL 侧完全相同。
--
-- 注意：本条只还原**数据库状态**。前端页面（views/monitor/log-storage-overview）已随 000038 的
-- up 一起删除，回滚这条迁移需要同时回滚那次前端改动，否则菜单指向一个不存在的组件。
-- SQL 幂等（按 path 条件判断）。
--
-- PG 侧差异：CURDATE() → CURRENT_DATE；is_expanded 的 0 → FALSE（PG 是 boolean）；
--   INSERT IGNORE → ON CONFLICT DO NOTHING。

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, create_time, update_time, remark, location, is_expanded)
SELECT '存储水位', 'dashboard', p.id, 1, '/monitor/logging/overview', 'monitor/log-storage-overview/index', 'C', '', CURRENT_DATE, CURRENT_DATE, 'data stream 运行态与磁盘水位', p.location, FALSE
FROM sys_menu p WHERE p.path = '/monitor/logging' AND p.menu_type = 'M'
  AND NOT EXISTS (SELECT 1 FROM sys_menu m WHERE m.path = '/monitor/logging/overview');

INSERT INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, m.id FROM sys_role_menu rm
JOIN sys_menu m ON m.path = '/monitor/logging/overview' AND m.menu_type = 'C'
WHERE rm.menu_id = (SELECT id FROM sys_menu WHERE path = '/monitor/logging' AND menu_type = 'M' LIMIT 1)
ON CONFLICT DO NOTHING;
