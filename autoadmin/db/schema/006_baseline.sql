-- OS 基线扫描（baseline 域）表快照。

CREATE TABLE `baseline` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `version` varchar(32) NOT NULL DEFAULT 'v1',
  `description` longtext NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
);

CREATE TABLE `baseline_category` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(64) NOT NULL,
  `sort` int unsigned NOT NULL DEFAULT 0,
  `baseline_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `baseline_category_baseline_fk` (`baseline_id`),
  CONSTRAINT `baseline_category_baseline_fk` FOREIGN KEY (`baseline_id`) REFERENCES `baseline` (`id`) ON DELETE CASCADE
);

CREATE TABLE `baseline_item` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `baseline_id` bigint NOT NULL,
  `category_id` bigint NOT NULL,
  `sort` int unsigned NOT NULL,
  `name` varchar(255) NOT NULL,
  `description` longtext NOT NULL,
  `config` json NOT NULL,
  `severity` varchar(16) NOT NULL DEFAULT 'high',
  PRIMARY KEY (`id`),
  KEY `baseline_item_baseline_fk` (`baseline_id`),
  CONSTRAINT `baseline_item_baseline_fk` FOREIGN KEY (`baseline_id`) REFERENCES `baseline` (`id`),
  KEY `baseline_item_category_fk` (`category_id`),
  CONSTRAINT `baseline_item_category_fk` FOREIGN KEY (`category_id`) REFERENCES `baseline_category` (`id`)
);

CREATE TABLE `security_scan` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `scan_type` varchar(16) NOT NULL DEFAULT 'baseline',
  `baseline_id` bigint NOT NULL,
  `mount_type` varchar(16) NOT NULL,
  `project_id` bigint DEFAULT NULL,
  `environment_id` bigint DEFAULT NULL,
  `status` varchar(16) NOT NULL DEFAULT 'pending',
  `summary` json NOT NULL,
  `requested_username` varchar(100) NOT NULL,
  `start_time` datetime(6) DEFAULT NULL,
  `end_time` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `security_scan_baseline_fk` (`baseline_id`),
  CONSTRAINT `security_scan_baseline_fk` FOREIGN KEY (`baseline_id`) REFERENCES `baseline` (`id`)
);

CREATE TABLE `security_scan_target` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `scan_id` bigint NOT NULL,
  `host_id` bigint NOT NULL,
  `host_name` varchar(255) NOT NULL,
  `host_ip` varchar(64) NOT NULL,
  `agent_id` varchar(128) NOT NULL,
  `status` varchar(16) NOT NULL DEFAULT 'pending',
  `passed_items` int NOT NULL DEFAULT 0,
  `failed_items` int NOT NULL DEFAULT 0,
  `compliance_rate` decimal(5,2) NOT NULL DEFAULT 0,
  `error_message` longtext NOT NULL,
  PRIMARY KEY (`id`),
  KEY `security_scan_target_scan_fk` (`scan_id`),
  CONSTRAINT `security_scan_target_scan_fk` FOREIGN KEY (`scan_id`) REFERENCES `security_scan` (`id`)
);

CREATE TABLE `baseline_scan_result` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `scan_id` bigint NOT NULL,
  `host_id` bigint NOT NULL,
  `item_id` bigint NOT NULL,
  `item_name` varchar(255) NOT NULL,
  `chapter` varchar(64) NOT NULL,
  `severity` varchar(16) NOT NULL,
  `status` varchar(16) NOT NULL,
  `expected_value` json DEFAULT NULL,
  `actual_value` json DEFAULT NULL,
  `message` longtext NOT NULL,
  `remediation` longtext,
  PRIMARY KEY (`id`),
  KEY `baseline_scan_result_scan_fk` (`scan_id`),
  CONSTRAINT `baseline_scan_result_scan_fk` FOREIGN KEY (`scan_id`) REFERENCES `security_scan` (`id`)
);
