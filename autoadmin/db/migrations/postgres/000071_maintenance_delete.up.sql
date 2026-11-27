INSERT INTO sys_menu (name,icon,parent_id,order_num,path,component,menu_type,perms,location,is_expanded,create_time,update_time,remark)
SELECT '删除维护窗口','',p.id,0,'','','F','monitor:maintenance:delete',p.location,FALSE,CURRENT_DATE,CURRENT_DATE,'migration:000071:maintenance'
FROM sys_menu p WHERE p.perms='monitor:maintenance:view';

INSERT INTO sys_role_menu (role_id,menu_id)
SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.code='admin' AND m.remark='migration:000071:maintenance';
