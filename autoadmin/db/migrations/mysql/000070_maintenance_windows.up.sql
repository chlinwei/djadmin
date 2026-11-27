CREATE INDEX maintenance_delivery_pending ON monitor_alert_notification_event (status, id, update_time);
CREATE TABLE monitor_maintenance_window (
  `id` bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `title` varchar(200) NOT NULL,
  `reason` text NOT NULL,
  `starts_at` datetime(6) NOT NULL,
  `ends_at` datetime(6) NOT NULL,
  `matchers` json NOT NULL,
  `status` varchar(16) NOT NULL,
  `created_by` varchar(150) NOT NULL,
  `updated_by` varchar(150) NOT NULL,
  `created_at` datetime(6) NOT NULL,
  `updated_at` datetime(6) NOT NULL,
  KEY maintenance_active (status, ends_at, starts_at)
);
CREATE TABLE monitor_maintenance_window_audit (
  `id` bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `window_id` bigint NOT NULL,
  `action` varchar(16) NOT NULL,
  `actor` varchar(150) NOT NULL,
  `decided_at` datetime(6) NOT NULL,
  `snapshot` json NOT NULL,
  FOREIGN KEY (window_id) REFERENCES monitor_maintenance_window(id)
);
CREATE TABLE monitor_maintenance_suppression (
  `event_id` bigint NOT NULL,
  `window_id` bigint NOT NULL,
  `title` varchar(255) NOT NULL,
  `ends_at` datetime(6) NOT NULL,
  `decided_at` datetime(6) NOT NULL,
  PRIMARY KEY (event_id, window_id),
  KEY maintenance_suppression_window (window_id),
  FOREIGN KEY (event_id) REFERENCES monitor_alert_notification_event(id) ON DELETE CASCADE
);
CREATE TABLE monitor_notification_intent (
  `alert_id` bigint NOT NULL,
  `event_type` varchar(16) NOT NULL,
  PRIMARY KEY (alert_id, event_type),
  FOREIGN KEY (alert_id) REFERENCES monitor_alert_history(id) ON DELETE CASCADE
);

INSERT INTO sys_menu (name,icon,parent_id,order_num,path,component,menu_type,perms,location,is_expanded,create_time,update_time,remark)
SELECT '维护窗口','calendar',p.id,3,'/monitor/notification/maintenance','monitor/maintenance/index','C','monitor:maintenance:view',p.location,FALSE,CURRENT_DATE,CURRENT_DATE,'migration:000070:maintenance'
FROM sys_menu p WHERE p.path='/monitor/notification' AND p.menu_type='M';

INSERT INTO sys_menu (name,icon,parent_id,order_num,path,component,menu_type,perms,location,is_expanded,create_time,update_time,remark)
SELECT a.name,'',p.id,0,'','','F',a.perms,p.location,FALSE,CURRENT_DATE,CURRENT_DATE,'migration:000070:maintenance'
FROM sys_menu p CROSS JOIN (
 SELECT '创建维护窗口' AS name,'monitor:maintenance:create' AS perms
 UNION ALL SELECT '修改维护窗口','monitor:maintenance:update'
 UNION ALL SELECT '取消维护窗口','monitor:maintenance:cancel'
 UNION ALL SELECT '提前结束维护窗口','monitor:maintenance:end'
) a WHERE p.perms='monitor:maintenance:view';

-- 写权限独立授权，不按已有查看权自动补授。
INSERT INTO sys_role_menu (role_id,menu_id)
SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.code='admin' AND m.remark='migration:000070:maintenance';
