-- Snapshot source: Django migrations for the monitor and inspection tables used by typed Go queries.

CREATE TABLE `monitor_alert_route` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `matchers` json NOT NULL,
  `notify_on_firing` BOOLEAN NOT NULL,
  `notify_on_resolved` BOOLEAN NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
);

CREATE TABLE `inspection_target_execution` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `execution_id` bigint NOT NULL,
  `deployment_id` bigint DEFAULT NULL,
  `host_id` bigint DEFAULT NULL,
  `target_name` varchar(255) NOT NULL,
  `host_id_snapshot` int DEFAULT NULL,
  `host_ip_snapshot` varchar(64) NOT NULL,
  `agent_id_snapshot` varchar(128) NOT NULL,
  `status` varchar(16) NOT NULL,
  `passed` BOOLEAN DEFAULT NULL,
  `error_message` longtext NOT NULL,
  `raw_result` json NOT NULL,
  `start_time` datetime(6) DEFAULT NULL,
  `end_time` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`id`)
);

-- Snapshot source: `SHOW CREATE TABLE monitor_target` against the live migration database
-- (tinyint(1) columns normalized to BOOLEAN per this directory's README so sqlc emits real `bool`).
CREATE TABLE `monitor_target` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `exporter_type` varchar(64) NOT NULL,
  `managed_enabled` BOOLEAN NOT NULL,
  `install_status` varchar(16) NOT NULL,
  `install_message` longtext NOT NULL,
  `last_scrape_status` varchar(16) NOT NULL,
  `last_scrape_at` datetime(6) DEFAULT NULL,
  `labels` json NOT NULL,
  `host_id` bigint NOT NULL,
  `retry_count` int unsigned NOT NULL,
  `last_dispatch_manual` BOOLEAN NOT NULL,
  `scrape_port` int unsigned NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `monitor_target_host_id_exporter_type_dfe9c277_uniq` (`host_id`,`exporter_type`),
  CONSTRAINT `monitor_target_host_fk` FOREIGN KEY (`host_id`) REFERENCES `assets_host` (`id`)
);

CREATE TABLE `monitor_log_retention_tier` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `code` varchar(32) NOT NULL,
  `name` varchar(64) NOT NULL,
  `daily_size_gb` double NOT NULL,
  `retention_days` int unsigned NOT NULL,
  `rollover_min_index_age` varchar(16) NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `is_default` BOOLEAN NOT NULL,
  `remark` longtext NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `code` (`code`)
);

CREATE TABLE `monitor_opensearch_cluster` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `name` varchar(128) NOT NULL,
  `hosts` varchar(512) NOT NULL,
  `username` varchar(128) NOT NULL,
  `password` varchar(512) NOT NULL,
  `verify_tls` BOOLEAN NOT NULL,
  `ca_cert` longtext NOT NULL,
  `index_prefix` varchar(64) NOT NULL,
  `request_timeout` int unsigned NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `is_default` BOOLEAN NOT NULL,
  `last_check_time` datetime(6) DEFAULT NULL,
  `last_check_success` BOOLEAN DEFAULT NULL,
  `last_check_message` longtext NOT NULL,
  `remark` longtext NOT NULL,
  `storage_sync_error` longtext NOT NULL,
  `storage_sync_status` varchar(16) NOT NULL,
  `storage_sync_time` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
);

CREATE TABLE `monitor_log_processing_rule` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `description` varchar(500) NOT NULL,
  `input_format` varchar(16) NOT NULL,
  `multiline_enabled` BOOLEAN NOT NULL,
  `start_pattern` longtext NOT NULL,
  `continuation_pattern` longtext NOT NULL,
  `flush_timeout` int unsigned NOT NULL,
  `pipeline_body` json NOT NULL,
  `cluster_id` bigint NOT NULL,
  `application_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`),
  CONSTRAINT `monitor_log_processing_rule_cluster_fk` FOREIGN KEY (`cluster_id`) REFERENCES `monitor_opensearch_cluster` (`id`)
);

CREATE TABLE `monitor_log_collection_filter_rule` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `description` varchar(500) NOT NULL,
  `pattern` longtext NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `application_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
);

-- 只声明 monitor_software_package 的 JOIN 目标够用的列，不是 automation 包的完整基线。
CREATE TABLE `automation_playbook_template` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `description` varchar(255) NOT NULL,
  `content` longtext NOT NULL,
  `category` varchar(32) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
);

CREATE TABLE `monitor_software_package` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(64) NOT NULL,
  `version` varchar(32) NOT NULL,
  `os` varchar(16) NOT NULL,
  `arch` varchar(16) NOT NULL,
  `file` varchar(255) NOT NULL,
  `sha256` varchar(64) NOT NULL,
  `size_bytes` bigint NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `service_file_content` longtext NOT NULL,
  `service_run_as_group` varchar(100) NOT NULL,
  `service_run_as_user` varchar(100) NOT NULL,
  `install_playbook_template_id` bigint DEFAULT NULL,
  `uninstall_playbook_template_id` bigint DEFAULT NULL,
  `work_directory` varchar(255) NOT NULL,
  `default_port` int unsigned NOT NULL,
  `package_format` varchar(16) NOT NULL,
  `platform_family` varchar(16) NOT NULL,
  `platform_major` varchar(16) NOT NULL,
  `package_type` varchar(16) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `monitor_software_package_uniq` (`package_type`,`name`,`version`,`os`,`arch`,`platform_family`,`platform_major`),
  CONSTRAINT `monitor_software_package_install_fk` FOREIGN KEY (`install_playbook_template_id`) REFERENCES `automation_playbook_template` (`id`),
  CONSTRAINT `monitor_software_package_uninstall_fk` FOREIGN KEY (`uninstall_playbook_template_id`) REFERENCES `automation_playbook_template` (`id`)
);

CREATE TABLE `monitor_target_install_history` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `action` varchar(16) NOT NULL,
  `trigger_type` varchar(16) NOT NULL,
  `status` varchar(16) NOT NULL,
  `host_id_snapshot` int DEFAULT NULL,
  `host_name_snapshot` varchar(128) NOT NULL,
  `host_ip_snapshot` varchar(64) NOT NULL,
  `exporter_type_snapshot` varchar(64) NOT NULL,
  `summary_message` longtext NOT NULL,
  `stdout_snapshot` longtext NOT NULL,
  `stderr_snapshot` longtext NOT NULL,
  `error_message_snapshot` longtext NOT NULL,
  `result_summary_snapshot` json NOT NULL,
  `requested_user_id_snapshot` int DEFAULT NULL,
  `requested_username_snapshot` varchar(100) NOT NULL,
  `start_time` datetime(6) DEFAULT NULL,
  `end_time` datetime(6) DEFAULT NULL,
  `duration_seconds` double DEFAULT NULL,
  `host_id` bigint DEFAULT NULL,
  `target_id` bigint DEFAULT NULL,
  `log_collection_target_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `monitor_target_install_history_target_fk` FOREIGN KEY (`target_id`) REFERENCES `monitor_target` (`id`)
);

CREATE TABLE `monitor_alert_history` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `fingerprint` varchar(40) NOT NULL,
  `alertname` varchar(200) NOT NULL,
  `severity` varchar(32) NOT NULL,
  `instance` varchar(200) NOT NULL,
  `labels` json NOT NULL,
  `annotations` json NOT NULL,
  `generator_url` varchar(500) NOT NULL,
  `state` varchar(16) NOT NULL,
  `started_at` datetime(6) NOT NULL,
  `resolved_at` datetime(6) DEFAULT NULL,
  `last_seen_at` datetime(6) NOT NULL,
  `resolved_by_reconciliation` BOOLEAN NOT NULL,
  `rule_group` varchar(200) NOT NULL,
  `rule_snapshot` json NOT NULL,
  `source` varchar(16) NOT NULL,
  PRIMARY KEY (`id`)
);

CREATE TABLE `monitor_alert_notification_event` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `event_type` varchar(16) NOT NULL,
  `deduplication_key` varchar(255) NOT NULL,
  `status` varchar(16) NOT NULL,
  `attempt_count` int unsigned NOT NULL,
  `error_message` longtext NOT NULL,
  `sent_at` datetime(6) DEFAULT NULL,
  `alert_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `deduplication_key` (`deduplication_key`),
  CONSTRAINT `monitor_alert_notification_event_alert_fk` FOREIGN KEY (`alert_id`) REFERENCES `monitor_alert_history` (`id`)
);

CREATE TABLE `monitor_alert_notification_delivery` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `address` varchar(500) NOT NULL,
  `status` varchar(16) NOT NULL,
  `attempt_count` int unsigned NOT NULL,
  `error_message` longtext NOT NULL,
  `sent_at` datetime(6) DEFAULT NULL,
  `event_id` bigint NOT NULL,
  `media_id` bigint DEFAULT NULL,
  `user_id` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `monitor_alert_delivery_uniq` (`event_id`,`media_id`,`user_id`,`address`),
  CONSTRAINT `monitor_alert_notification_delivery_event_fk` FOREIGN KEY (`event_id`) REFERENCES `monitor_alert_notification_event` (`id`),
  CONSTRAINT `monitor_alert_notification_delivery_media_fk` FOREIGN KEY (`media_id`) REFERENCES `monitor_alert_media` (`id`),
  CONSTRAINT `monitor_alert_notification_delivery_user_fk` FOREIGN KEY (`user_id`) REFERENCES `sys_user` (`id`)
);