-- 与 MySQL 使用相同标准 SQL，无语句级方言差异。
-- 先清引用再删本迁移新建的 F 节点；不撤销其他旧权限或迁移前已存在的同码节点。
DELETE FROM sys_role_menu WHERE menu_id IN (
    SELECT id FROM sys_menu
    WHERE menu_type = 'F' AND remark = 'migration:000061:readonly_permissions'
      AND perms IN (
          'assets:hosts:execute', 'system:scheduler:update', 'system:scheduler:execute', 'sys:config:update',
          'monitor:create', 'monitor:update', 'monitor:delete', 'monitor:execute', 'monitor:logs:delete',
          'monitor:log_collect:create', 'monitor:log_collect:execute', 'monitor:log_collect:delete'
      )
);

DELETE FROM sys_menu
WHERE menu_type = 'F' AND remark = 'migration:000061:readonly_permissions'
  AND perms IN (
      'assets:hosts:execute', 'system:scheduler:update', 'system:scheduler:execute', 'sys:config:update',
      'monitor:create', 'monitor:update', 'monitor:delete', 'monitor:execute', 'monitor:logs:delete',
      'monitor:log_collect:create', 'monitor:log_collect:execute', 'monitor:log_collect:delete'
  );

-- 只恢复页面的旧权限码；manage/scan 按钮与它们的既有角色授权均保持不变。
UPDATE sys_menu SET perms = 'baseline:manage', update_time = CURRENT_DATE
WHERE menu_type = 'C' AND perms = 'baseline:view';
