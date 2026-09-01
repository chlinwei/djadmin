-- Snapshot source: SHOW CREATE TABLE against the fully migrated Django MySQL database.
-- AUTO_INCREMENT counters are intentionally omitted because they are data, not schema.

CREATE TABLE `sys_role` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(30) DEFAULT NULL,
  `code` varchar(100) DEFAULT NULL,
  `create_time` date DEFAULT NULL,
  `update_time` date DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `sys_user` (
  `id` int NOT NULL AUTO_INCREMENT,
  `username` varchar(100) NOT NULL,
  `password` varchar(100) NOT NULL,
  `avatar` varchar(255) DEFAULT NULL,
  `phonenumber` varchar(11) DEFAULT NULL,
  `login_date` datetime(6) DEFAULT NULL,
  `status` smallint NOT NULL,
  `create_time` date DEFAULT NULL,
  `update_time` date DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  `timezone` varchar(50) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `sys_menu` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(50) NOT NULL,
  `icon` varchar(100) DEFAULT NULL,
  `parent_id` int DEFAULT NULL,
  `order_num` int DEFAULT NULL,
  `path` varchar(200) DEFAULT NULL,
  `component` varchar(255) DEFAULT NULL,
  `menu_type` varchar(1) DEFAULT NULL,
  `perms` varchar(100) DEFAULT NULL,
  `create_time` date DEFAULT NULL,
  `update_time` date DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  `location` smallint NOT NULL,
  `is_expanded` tinyint(1) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `sys_user_role` (
  `id` int NOT NULL AUTO_INCREMENT,
  `role_id` int NOT NULL,
  `user_id` int NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `sys_user_role_user_id_role_id_ec6ef495_uniq` (`user_id`,`role_id`),
  KEY `sys_user_role_role_id_63624973_fk_sys_role_id` (`role_id`),
  CONSTRAINT `sys_user_role_role_id_63624973_fk_sys_role_id` FOREIGN KEY (`role_id`) REFERENCES `sys_role` (`id`),
  CONSTRAINT `sys_user_role_user_id_5f2fb964_fk_sys_user_id` FOREIGN KEY (`user_id`) REFERENCES `sys_user` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `sys_role_menu` (
  `id` int NOT NULL AUTO_INCREMENT,
  `menu_id` int NOT NULL,
  `role_id` int NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `sys_role_menu_menu_id_role_id_607d95aa_uniq` (`menu_id`,`role_id`),
  KEY `sys_role_menu_role_id_e0dcb43b_fk_sys_role_id` (`role_id`),
  CONSTRAINT `sys_role_menu_menu_id_5c7ca896_fk_menu_sysmenu_id` FOREIGN KEY (`menu_id`) REFERENCES `sys_menu` (`id`),
  CONSTRAINT `sys_role_menu_role_id_e0dcb43b_fk_sys_role_id` FOREIGN KEY (`role_id`) REFERENCES `sys_role` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `sys_agent_token` (
  `id` int NOT NULL AUTO_INCREMENT,
  `agent_id` varchar(128) NOT NULL,
  `token_hash` varchar(255) NOT NULL,
  `name` varchar(128) DEFAULT NULL,
  `is_active` tinyint(1) NOT NULL,
  `expires_at` datetime(6) DEFAULT NULL,
  `last_used_at` datetime(6) DEFAULT NULL,
  `remark` varchar(500) DEFAULT NULL,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `created_by_id` int DEFAULT NULL,
  `bind_mode` varchar(32) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `sys_agent_token_created_by_id_526bb742_fk_sys_user_id` (`created_by_id`),
  CONSTRAINT `sys_agent_token_created_by_id_526bb742_fk_sys_user_id` FOREIGN KEY (`created_by_id`) REFERENCES `sys_user` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `monitor_alert_media` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `media_type` varchar(16) NOT NULL,
  `config` json NOT NULL,
  `enabled` tinyint(1) NOT NULL,
  `recipients` json NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `monitor_alert_media_name_uniq` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `sys_user_alert_media` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `sysuser_id` int NOT NULL,
  `alertmedia_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `sys_user_alert_media_sysuser_id_alertmedia_id_5ab059c5_uniq` (`sysuser_id`,`alertmedia_id`),
  KEY `sys_user_alert_media_alertmedia_id_c286c9b1_fk_monitor_a` (`alertmedia_id`),
  CONSTRAINT `sys_user_alert_media_alertmedia_id_c286c9b1_fk_monitor_a` FOREIGN KEY (`alertmedia_id`) REFERENCES `monitor_alert_media` (`id`),
  CONSTRAINT `sys_user_alert_media_sysuser_id_29476095_fk_sys_user_id` FOREIGN KEY (`sysuser_id`) REFERENCES `sys_user` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `sys_config` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `key` varchar(128) NOT NULL,
  `value` longtext NOT NULL,
  `value_type` varchar(16) NOT NULL,
  `name` varchar(128) NOT NULL,
  `description` longtext,
  `is_readonly` tinyint(1) NOT NULL,
  `default_value` longtext,
  PRIMARY KEY (`id`),
  UNIQUE KEY `key` (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `audit_login_log` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `username` varchar(150) NOT NULL,
  `user_id` int DEFAULT NULL,
  `status` varchar(16) NOT NULL,
  `client_ip` varchar(64) NOT NULL,
  `user_agent` varchar(255) NOT NULL,
  `message` varchar(255) NOT NULL,
  `login_time` datetime(6) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `audit_operation_log` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `username` varchar(150) NOT NULL,
  `user_id` int DEFAULT NULL,
  `method` varchar(16) NOT NULL,
  `path` varchar(255) NOT NULL,
  `route_name` varchar(255) NOT NULL,
  `client_ip` varchar(64) NOT NULL,
  `user_agent` varchar(255) NOT NULL,
  `status_code` int NOT NULL,
  `duration_ms` int DEFAULT NULL,
  `message` varchar(255) NOT NULL,
  `created_at` datetime(6) NOT NULL,
  `request_data` longtext NOT NULL,
  `response_data` longtext NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `scheduler_scheduledtask` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `code` varchar(128) NOT NULL,
  `description` varchar(255) DEFAULT NULL,
  `enabled` tinyint(1) NOT NULL,
  `interval_minutes` int DEFAULT NULL,
  `last_run_time` datetime(6) DEFAULT NULL,
  `last_status` varchar(32) DEFAULT NULL,
  `last_message` longtext,
  `menu_id` int DEFAULT NULL,
  `next_run_time` datetime(6) DEFAULT NULL,
  `is_running` tinyint(1) NOT NULL,
  `cron_expression` varchar(120) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`),
  UNIQUE KEY `code` (`code`),
  KEY `scheduler_scheduledtask_menu_id_8b41dd60_fk_sys_menu_id` (`menu_id`),
  CONSTRAINT `scheduler_scheduledtask_menu_id_8b41dd60_fk_sys_menu_id` FOREIGN KEY (`menu_id`) REFERENCES `sys_menu` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `scheduler_scheduledtasklog` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `run_time` datetime(6) NOT NULL,
  `status` varchar(32) NOT NULL,
  `message` longtext,
  `duration_seconds` double DEFAULT NULL,
  `task_id` bigint NOT NULL,
  `output` longtext,
  PRIMARY KEY (`id`),
  KEY `scheduler_scheduledt_task_id_a41cf2ab_fk_scheduler` (`task_id`),
  CONSTRAINT `scheduler_scheduledt_task_id_a41cf2ab_fk_scheduler` FOREIGN KEY (`task_id`) REFERENCES `scheduler_scheduledtask` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `assets_webssh_session_log` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` int DEFAULT NULL,
  `username` varchar(100) NOT NULL,
  `client_ip` varchar(64) NOT NULL,
  `user_agent` varchar(255) NOT NULL,
  `status` varchar(16) NOT NULL,
  `start_time` datetime(6) NOT NULL,
  `end_time` datetime(6) DEFAULT NULL,
  `duration_seconds` int DEFAULT NULL,
  `close_code` int DEFAULT NULL,
  `error_message` longtext NOT NULL,
  `input_bytes` int NOT NULL,
  `command_count` int NOT NULL,
  `host_id` bigint NOT NULL,
  `input_content` longtext NOT NULL,
  `output_content` longtext NOT NULL,
  `recorded_content_bytes` int NOT NULL,
  `is_content_truncated` tinyint(1) NOT NULL,
  `effective_username` varchar(100) NOT NULL,
  `requested_username` varchar(100) NOT NULL,
  `switch_user_error` longtext NOT NULL,
  `switch_user_status` varchar(16) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `assets_webssh_session_log_host_id_71c2140d_fk_assets_host_id` (`host_id`),
  CONSTRAINT `assets_webssh_session_log_host_id_71c2140d_fk_assets_host_id` FOREIGN KEY (`host_id`) REFERENCES `assets_host` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;