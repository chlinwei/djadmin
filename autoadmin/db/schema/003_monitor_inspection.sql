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