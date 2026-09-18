-- 日志管理目录下新增「日志采集」菜单：Filebeat 纳管目标的独立页面。
-- 从「智能监控 → 纳管目标」拆出——那里是 exporter 与 Filebeat 混在同一张主机表里用 segmented 切换，
-- 而采集（安装 → 下发配置 → 看配置状态）与日志存储/处理规则/保留档位同属一条链路。
-- 该菜单带独立权限点 monitor:log_collect:view，采集类接口（/monitor/log-targets/*）据此鉴权。
-- SQL 幂等（按 path 条件判断）；目录节点本身无组件路由，仅作侧边栏分组。

INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, create_time, update_time, remark, location, is_expanded)
SELECT '日志采集', 'file-lines', p.id, 4, '/monitor/logging/collectors', 'monitor/log-collectors/index', 'C', 'monitor:log_collect:view', CURDATE(), CURDATE(), 'Filebeat 纳管目标：安装/启停/下发采集配置与配置状态', p.location, 0
FROM sys_menu p WHERE p.path = '/monitor/logging' AND p.menu_type = 'M'
  AND NOT EXISTS (SELECT 1 FROM sys_menu m WHERE m.path = '/monitor/logging/collectors');

-- 新增菜单补授权：给所有拥有父目录（日志管理）的角色授权（sys_role_menu 按菜单 id 授权，
-- 不补授则菜单树整枝不可见，且用户拿不到 monitor:log_collect:view → 采集接口 403）。
-- 注意：权限码随 JWT 签发，存量登录用户需重新登录后才能访问采集接口。
INSERT IGNORE INTO sys_role_menu (role_id, menu_id)
SELECT rm.role_id, m.id FROM sys_role_menu rm
JOIN sys_menu m ON m.path = '/monitor/logging/collectors' AND m.menu_type = 'C'
WHERE rm.menu_id = (SELECT id FROM sys_menu WHERE path = '/monitor/logging' AND menu_type = 'M' LIMIT 1);
