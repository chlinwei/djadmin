-- 与 MySQL 使用相同的标准 SQL：CURRENT_DATE、FALSE 与 NOT EXISTS 在两侧语义一致。
-- 查看权限不隐含写入或执行权限；新增 F 节点可在角色授权树中独立勾选。
-- 只为 code=admin 的角色补新写权限，以保持管理员前端入口；不为普通角色按 view/list 自动提权。
-- username=admin 的后端绕过不依赖角色，本迁移不创建用户、角色或兼容授权。
-- remark 标记本迁移新建的节点，down 不删除迁移前已存在的同码权限。
INSERT INTO sys_menu (name, icon, parent_id, order_num, path, component, menu_type, perms, create_time, update_time, remark, location, is_expanded)
SELECT t.name, NULL, p.id, t.order_num, '', NULL, 'F', t.perms, CURRENT_DATE, CURRENT_DATE,
       'migration:000061:readonly_permissions', p.location, FALSE
FROM (
    SELECT '主机终端执行' AS name, 'assets:hosts:execute' AS perms, 'assets:hosts:view' AS parent_perms, '' AS parent_component, 90 AS order_num
    UNION ALL SELECT '定时任务编辑', 'system:scheduler:update', 'system:scheduler:view', '', 90
    UNION ALL SELECT '定时任务执行', 'system:scheduler:execute', 'system:scheduler:view', '', 91
    UNION ALL SELECT '系统配置编辑', 'sys:config:update', 'sys:config:list', '', 90
    UNION ALL SELECT '监控新增', 'monitor:create', 'monitor:view', 'monitor/index', 90
    UNION ALL SELECT '监控编辑', 'monitor:update', 'monitor:view', 'monitor/index', 91
    UNION ALL SELECT '监控删除', 'monitor:delete', 'monitor:view', 'monitor/index', 92
    UNION ALL SELECT '监控执行', 'monitor:execute', 'monitor:view', 'monitor/index', 93
    UNION ALL SELECT '监控日志删除', 'monitor:logs:delete', 'monitor:view', 'monitor/log-center/index', 90
    UNION ALL SELECT '日志采集新增', 'monitor:log_collect:create', 'monitor:log_collect:view', 'monitor/log-collectors/index', 90
    UNION ALL SELECT '日志采集执行', 'monitor:log_collect:execute', 'monitor:log_collect:view', 'monitor/log-collectors/index', 91
    UNION ALL SELECT '日志采集删除', 'monitor:log_collect:delete', 'monitor:log_collect:view', 'monitor/log-collectors/index', 92
) t
JOIN sys_menu p ON p.id = (
    SELECT candidate.id FROM sys_menu candidate
    WHERE candidate.menu_type = 'C' AND candidate.perms = t.parent_perms
      AND (t.parent_component = '' OR candidate.component = t.parent_component)
    ORDER BY candidate.id LIMIT 1
)
WHERE NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.perms = t.perms);

-- 同一个 monitor:view 被多个页面复用：以上按组件选唯一可见 C 父节点，不对所有页面复制按钮。
INSERT INTO sys_role_menu (role_id, menu_id)
SELECT r.id, m.id FROM sys_role r
JOIN sys_menu m ON m.menu_type = 'F' AND m.remark = 'migration:000061:readonly_permissions'
WHERE r.code = 'admin'
  AND NOT EXISTS (SELECT 1 FROM sys_role_menu existing WHERE existing.role_id = r.id AND existing.menu_id = m.id);

-- C 型页面只控制读取，F 型 baseline:manage 继续控制写入，不能由旧页面查看授权反向补写。
UPDATE sys_menu SET perms = 'baseline:view', update_time = CURRENT_DATE
WHERE menu_type = 'C' AND perms = 'baseline:manage';

-- 已有 manage 按钮授权的角色补页面读取；原 C 菜单授权随原 id 自然变为只读，不增加写能力。
INSERT INTO sys_role_menu (role_id, menu_id)
SELECT DISTINCT rm.role_id, page.id FROM sys_role_menu rm
JOIN sys_menu granted ON granted.id = rm.menu_id AND granted.menu_type = 'F' AND granted.perms = 'baseline:manage'
JOIN sys_menu page ON page.id = granted.parent_id AND page.menu_type = 'C' AND page.perms = 'baseline:view'
WHERE NOT EXISTS (SELECT 1 FROM sys_role_menu existing WHERE existing.role_id = rm.role_id AND existing.menu_id = page.id);
