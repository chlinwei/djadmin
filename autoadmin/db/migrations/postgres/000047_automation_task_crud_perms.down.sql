-- 回滚任务 CRUD 权限点：先清授权再删菜单，并把「任务执行」改回原名（仅当 perms 仍是 jobs:create，幂等）。
DELETE FROM sys_role_menu WHERE menu_id IN (
    SELECT id FROM (SELECT id FROM sys_menu WHERE perms IN ('automation:tasks:create', 'automation:tasks:update', 'automation:tasks:delete')) t
);
DELETE FROM sys_menu WHERE perms IN ('automation:tasks:create', 'automation:tasks:update', 'automation:tasks:delete');
UPDATE sys_menu SET name = '任务创建', update_time = CURRENT_DATE
WHERE name = '任务执行' AND perms = 'automation:jobs:create';
