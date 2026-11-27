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
