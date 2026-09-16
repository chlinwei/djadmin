-- 日志管理目录下新增「存储水位」菜单：data stream 运行态展示（大小/docs/rollover/ISM/节点磁盘水位）。
-- SQL 幂等（按 path 条件判断）；目录节点本身无组件路由，仅作侧边栏分组。
--
-- PG 侧差异：CURDATE() → CURRENT_DATE；is_expanded 的 0 → FALSE（PG 是 boolean）；
--   INSERT IGNORE → ON CONFLICT DO NOTHING。

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, create_time, update_time, remark, location, is_expanded)
SELECT '存储水位', 'dashboard', p.id, 1, '/monitor/logging/overview', 'monitor/log-storage-overview/index', 'C', '', CURRENT_DATE, CURRENT_DATE, 'data stream 运行态与磁盘水位', p.location, FALSE
FROM sys_menu p WHERE p.path = '/monitor/logging' AND p.menu_type = 'M'
  AND NOT EXISTS (SELECT 1 FROM sys_menu m WHERE m.path = '/monitor/logging/overview');

-- 新增菜单补授权：给所有拥有父目录（日志管理）的角色授权（sys_role_menu 按菜单 id 授权，
-- 不补授则菜单树整枝不可见）。INSERT IGNORE 幂等。
INSERT INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, m.id FROM sys_role_menu rm
JOIN sys_menu m ON m.path = '/monitor/logging/overview' AND m.menu_type = 'C'
WHERE rm.menu_id = (SELECT id FROM sys_menu WHERE path = '/monitor/logging' AND menu_type = 'M' LIMIT 1)
ON CONFLICT DO NOTHING;
