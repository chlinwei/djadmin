DROP INDEX maintenance_delivery_pending ON monitor_alert_notification_event;
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE remark='migration:000070:maintenance');
DELETE FROM sys_menu WHERE remark='migration:000070:maintenance' AND menu_type='F';
DELETE FROM sys_menu WHERE remark='migration:000070:maintenance';
DROP TABLE monitor_maintenance_suppression;
DROP TABLE monitor_notification_intent;
DROP TABLE monitor_maintenance_window_audit;
DROP TABLE monitor_maintenance_window;
