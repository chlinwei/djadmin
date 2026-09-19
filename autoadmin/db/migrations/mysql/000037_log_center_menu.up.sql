-- 日志管理目录下新增「日志中心」菜单：左侧服务树 + 右侧三个 tab（日志配置 / 日志查询 / 本服务水位）。
--
-- 为什么合成一个页面：这三块原本分散在三处（存储水位页、服务树页的日志查询 tab、逻辑服务编辑弹窗的
-- 模板日志表），看同一个服务的日志要在它们之间来回跳，而它们本来就共享同一个服务上下文
-- （日志检索接口的 application_service_id 是必填，其余两块也都要服务维度）。
--
-- perms 取 monitor:view：页面主体是日志检索与水位（monitor 域），与同目录其他子菜单一致。
-- 「日志配置」tab 读的是 assets 域的 log-config（需要 assets:applications:view），权限不足时
-- 该 tab 会显示接口返回的提示，不影响另外两个 tab——这是把入口权限收敛在监控域、不放大可见范围的做法。
-- SQL 幂等（按 path 条件判断）；目录节点本身无组件路由，仅作侧边栏分组。

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, create_time, update_time, remark, location, is_expanded)
SELECT '日志中心', 'search', p.id, 5, '/monitor/logging/center', 'monitor/log-center/index', 'C', 'monitor:view', CURDATE(), CURDATE(), '服务树 + 日志配置/查询/本服务水位', p.location, 0
FROM sys_menu p WHERE p.path = '/monitor/logging' AND p.menu_type = 'M'
  AND NOT EXISTS (SELECT 1 FROM sys_menu m WHERE m.path = '/monitor/logging/center');

-- 新增菜单补授权：给所有拥有父目录（日志管理）的角色授权（sys_role_menu 按菜单 id 授权，
-- 不补授则菜单树整枝不可见）。INSERT IGNORE 幂等。
INSERT IGNORE INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, m.id FROM sys_role_menu rm
JOIN sys_menu m ON m.path = '/monitor/logging/center' AND m.menu_type = 'C'
WHERE rm.menu_id = (SELECT id FROM sys_menu WHERE path = '/monitor/logging' AND menu_type = 'M' LIMIT 1);
