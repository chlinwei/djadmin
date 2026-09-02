-- Snapshot source: `SHOW CREATE TABLE` against the live migration database for the inspection
-- domain tables used by typed Go queries. tinyint(1) columns normalized to BOOLEAN per
-- db/schema/README.md so sqlc emits real `bool` fields.

CREATE TABLE `inspection_group` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `scope` varchar(24) NOT NULL,
  `description` longtext NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
);

CREATE TABLE `inspection_check` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `executor` varchar(16) NOT NULL,
  `config` json NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `order` int unsigned NOT NULL,
  `group_id` bigint NOT NULL,
  `severity` varchar(16) NOT NULL,
  `execution_location` varchar(16) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `unique_inspection_group_check_name` (`group_id`,`name`),
  CONSTRAINT `inspection_check_group_fk` FOREIGN KEY (`group_id`) REFERENCES `inspection_group` (`id`)
);

CREATE TABLE `inspection_task` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `concurrency` int unsigned NOT NULL,
  `timeout_seconds` int unsigned NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `group_id` bigint NOT NULL,
  `logical_service_id` bigint DEFAULT NULL,
  `cron_expression` varchar(120) NOT NULL,
  `last_run_time` datetime(6) DEFAULT NULL,
  `next_run_time` datetime(6) DEFAULT NULL,
  `inspection_name` varchar(128) NOT NULL,
  `selected_host_ids` json NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`),
  CONSTRAINT `inspection_task_group_fk` FOREIGN KEY (`group_id`) REFERENCES `inspection_group` (`id`)
);

CREATE TABLE `inspection_execution` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `status` varchar(16) NOT NULL,
  `task_snapshot` json NOT NULL,
  `group_snapshot` json NOT NULL,
  `service_snapshot` json NOT NULL,
  `target_snapshot` json NOT NULL,
  `summary` json NOT NULL,
  `requested_user_id` int DEFAULT NULL,
  `requested_username` varchar(100) NOT NULL,
  `start_time` datetime(6) DEFAULT NULL,
  `end_time` datetime(6) DEFAULT NULL,
  `task_id` bigint DEFAULT NULL,
  `trigger_type` varchar(16) NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `inspection_execution_task_fk` FOREIGN KEY (`task_id`) REFERENCES `inspection_task` (`id`)
);

CREATE TABLE `inspection_result` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `check_key` varchar(255) NOT NULL,
  `check_type` varchar(64) NOT NULL,
  `name` varchar(255) NOT NULL,
  `status` varchar(16) NOT NULL,
  `expected_value` json DEFAULT NULL,
  `actual_value` json DEFAULT NULL,
  `message` longtext NOT NULL,
  `target_id` bigint NOT NULL,
  `severity` varchar(16) NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `inspection_result_target_fk` FOREIGN KEY (`target_id`) REFERENCES `inspection_target_execution` (`id`)
);
