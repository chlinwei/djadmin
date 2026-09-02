-- Snapshot source: `SHOW CREATE TABLE` against the live migration database for the automation
-- domain tables used by typed Go queries. tinyint(1) columns normalized to BOOLEAN per
-- db/schema/README.md so sqlc emits real `bool` fields.

CREATE TABLE `automation_inventory` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `selected_host_ids` json NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `last_sync_host_count` int unsigned NOT NULL,
  `last_sync_message` longtext NOT NULL,
  `last_sync_status` varchar(16) NOT NULL,
  `last_sync_time` datetime(6) DEFAULT NULL,
  `update_cache_timeout` int unsigned NOT NULL,
  `update_on_launch` BOOLEAN NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
);

CREATE TABLE `automation_task` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `env_vars` json NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `inventory_id` bigint DEFAULT NULL,
  `default_limit` varchar(255) NOT NULL,
  `execution_timeout_seconds` int unsigned NOT NULL,
  `playbook_template_id` bigint DEFAULT NULL,
  `run_as_user` varchar(100) NOT NULL,
  `run_as_group` varchar(100) NOT NULL,
  `work_directory` varchar(255) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`),
  CONSTRAINT `automation_task_inventory_fk` FOREIGN KEY (`inventory_id`) REFERENCES `automation_inventory` (`id`),
  CONSTRAINT `automation_task_playbook_template_fk` FOREIGN KEY (`playbook_template_id`) REFERENCES `automation_playbook_template` (`id`)
);

CREATE TABLE `automation_execution_job` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `job_id` varchar(36) NOT NULL,
  `status` varchar(16) NOT NULL,
  `trigger_type` varchar(16) NOT NULL,
  `inventory_snapshot` json NOT NULL,
  `extra_vars` json NOT NULL,
  `result_summary` json NOT NULL,
  `requested_user_id` int DEFAULT NULL,
  `requested_username` varchar(100) NOT NULL,
  `start_time` datetime(6) DEFAULT NULL,
  `end_time` datetime(6) DEFAULT NULL,
  `duration_seconds` double DEFAULT NULL,
  `task_id` bigint DEFAULT NULL,
  `template_content_snapshot` longtext NOT NULL,
  `task_name_snapshot` varchar(128) NOT NULL,
  `template_name_snapshot` varchar(128) NOT NULL,
  `limit` varchar(255) NOT NULL,
  `run_as_user_snapshot` varchar(100) NOT NULL,
  `run_as_group_snapshot` varchar(100) NOT NULL,
  `work_directory_snapshot` varchar(255) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `job_id` (`job_id`),
  CONSTRAINT `automation_execution_job_task_fk` FOREIGN KEY (`task_id`) REFERENCES `automation_task` (`id`)
);
