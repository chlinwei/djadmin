-- Snapshot source: SHOW CREATE TABLE against the fully migrated Django MySQL database.
-- Only the static asset-management tables owned by this Go slice are included.

CREATE TABLE `assets_project` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `code` varchar(64) NOT NULL,
  `owner` varchar(128) NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`),
  UNIQUE KEY `code` (`code`)
);

CREATE TABLE `assets_business_system` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `code` varchar(64) NOT NULL,
  `owner` varchar(128) NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `project_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `unique_project_business_system_name` (`project_id`,`name`),
  UNIQUE KEY `unique_project_business_system_code` (`project_id`,`code`),
  CONSTRAINT `assets_business_system_project_fk` FOREIGN KEY (`project_id`) REFERENCES `assets_project` (`id`)
);

CREATE TABLE `assets_business_environment` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(64) NOT NULL,
  `code` varchar(32) NOT NULL,
  `order` int unsigned NOT NULL,
  `owner` varchar(128) NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `unique_business_environment_code` (`code`),
  UNIQUE KEY `unique_business_environment_name` (`name`)
);

CREATE TABLE `assets_credential` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(200) DEFAULT NULL,
  `password` varchar(512) DEFAULT NULL,
  `private_key` longtext,
  `auth_type` int NOT NULL,
  `username` varchar(128) NOT NULL,
  `port` int unsigned NOT NULL,
  PRIMARY KEY (`id`)
);

CREATE TABLE `assets_hostgroup` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `parent_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`),
  CONSTRAINT `assets_hostgroup_parent_fk` FOREIGN KEY (`parent_id`) REFERENCES `assets_hostgroup` (`id`)
);

CREATE TABLE `assets_host` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `status` varchar(32) NOT NULL,
  `instance_id` varchar(128) DEFAULT NULL,
  `ip` char(39) DEFAULT NULL,
  `is_deleted_in_cloud` BOOLEAN NOT NULL,
  `cloud_account_id` bigint DEFAULT NULL,
  `group_id` bigint DEFAULT NULL,
  `instance_name` varchar(128) DEFAULT NULL,
  `collect_status` varchar(16) NOT NULL,
  `collect_message` longtext NOT NULL,
  `collect_time` datetime(6) DEFAULT NULL,
  `agent_online` BOOLEAN NOT NULL,
  `agent_online_time` datetime(6) DEFAULT NULL,
  `webssh_default_username` varchar(100) NOT NULL,
  `webssh_login_users` varchar(512) NOT NULL,
  `environment_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  -- instance_name 是主机的全局唯一标识（agent 侧 DJ_AGENT_INSTANCE_NAME），
  -- 网关握手落库按它 UPDATE，必须有索引且必须有 DB 级唯一保证：
  -- 原 agent_id 列带 UNIQUE KEY，该列删除后唯一约束不能丢。
  UNIQUE KEY `assets_host_instance_name_uniq` (`instance_name`),
  -- ip 是寻址标识：服务层按它做唯一性校验（SELECT COUNT(*) WHERE ip=?），
  -- 且主机搜索/按 IP 定位都走它，同样需要索引。唯一性当前仍由服务层保证
  -- （保留存量重复数据平滑收敛的余地），故此处只建普通索引。
  KEY `assets_host_ip_idx` (`ip`),
  KEY `assets_host_group_id_idx` (`group_id`),
  KEY `assets_host_environment_id_idx` (`environment_id`),
  CONSTRAINT `assets_host_group_fk` FOREIGN KEY (`group_id`) REFERENCES `assets_hostgroup` (`id`),
  CONSTRAINT `assets_host_environment_fk` FOREIGN KEY (`environment_id`) REFERENCES `assets_business_environment` (`id`)
);

CREATE TABLE `assets_hostcredential` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `is_default` BOOLEAN NOT NULL,
  `credential_id` bigint NOT NULL,
  `host_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `assets_hostcredential_host_credential_uniq` (`host_id`,`credential_id`),
  CONSTRAINT `assets_hostcredential_credential_fk` FOREIGN KEY (`credential_id`) REFERENCES `assets_credential` (`id`),
  CONSTRAINT `assets_hostcredential_host_fk` FOREIGN KEY (`host_id`) REFERENCES `assets_host` (`id`)
);

CREATE TABLE `assets_application` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `category` varchar(32) NOT NULL,
  `code` varchar(64) NOT NULL,
  `description` longtext NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `vendor` varchar(128) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `code` (`code`),
  UNIQUE KEY `assets_application_name_efca0489_uniq` (`name`)
);

CREATE TABLE `assets_application_version` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `version` varchar(128) NOT NULL,
  `release_date` date DEFAULT NULL,
  `end_of_support` date DEFAULT NULL,
  `enabled` BOOLEAN NOT NULL,
  `application_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `unique_application_version` (`application_id`,`version`),
  CONSTRAINT `assets_application_version_application_fk` FOREIGN KEY (`application_id`) REFERENCES `assets_application` (`id`)
);

CREATE TABLE `assets_cluster_profile` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `code` varchar(64) NOT NULL,
  `profile_type` varchar(16) NOT NULL,
  `enabled` BOOLEAN NOT NULL,
  `application_id` bigint DEFAULT NULL,
  `cluster_type` varchar(24) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`),
  UNIQUE KEY `code` (`code`),
  CONSTRAINT `assets_cluster_profile_application_fk` FOREIGN KEY (`application_id`) REFERENCES `assets_application` (`id`)
);

CREATE TABLE `assets_application_deployment_template` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL,
  `remark` longtext, `name` varchar(128) NOT NULL, `control_type` varchar(32) NOT NULL, `run_user` varchar(100) NOT NULL,
  `run_group` varchar(100) NOT NULL, `app_home` varchar(512) NOT NULL, `work_directory` varchar(512) NOT NULL,
  `service_name` varchar(255) NOT NULL, `ha_system_name` varchar(128) NOT NULL, `ha_cluster_name` varchar(128) NOT NULL,
  `ha_resource_name` varchar(128) NOT NULL, `enabled` BOOLEAN NOT NULL, `application_id` bigint NOT NULL,
  `systemd_scope` varchar(16) NOT NULL, `macro_definitions` json NOT NULL, PRIMARY KEY (`id`),
  UNIQUE KEY `unique_application_deployment_template` (`application_id`,`name`),
  CONSTRAINT `assets_application_deployment_template_application_fk` FOREIGN KEY (`application_id`) REFERENCES `assets_application` (`id`)
);

CREATE TABLE `assets_application_port` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `name` varchar(64) NOT NULL, `protocol` varchar(8) NOT NULL, `bind_address` varchar(255) NOT NULL, `port` int unsigned NOT NULL,
  `required` BOOLEAN NOT NULL, `external_access` BOOLEAN NOT NULL, `check_enabled` BOOLEAN NOT NULL,
  `deployment_template_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE KEY `unique_template_protocol_port` (`deployment_template_id`,`protocol`,`port`),
  CONSTRAINT `assets_application_port_template_fk` FOREIGN KEY (`deployment_template_id`) REFERENCES `assets_application_deployment_template` (`id`)
);

CREATE TABLE `assets_application_path` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `name` varchar(64) NOT NULL, `path_type` varchar(16) NOT NULL, `path` varchar(512) NOT NULL, `required` BOOLEAN NOT NULL,
  `expected_owner` varchar(100) NOT NULL, `expected_group` varchar(100) NOT NULL, `expected_mode` varchar(8) NOT NULL, `check_enabled` BOOLEAN NOT NULL,
  `deployment_template_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE KEY `unique_template_path_name` (`deployment_template_id`,`name`),
  CONSTRAINT `assets_application_path_template_fk` FOREIGN KEY (`deployment_template_id`) REFERENCES `assets_application_deployment_template` (`id`)
);

CREATE TABLE `assets_application_config_file` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `name` varchar(128) NOT NULL, `path` varchar(512) NOT NULL, `file_format` varchar(16) NOT NULL, `required` BOOLEAN NOT NULL,
  `deployment_template_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE KEY `unique_template_config_path` (`deployment_template_id`,`path`),
  CONSTRAINT `assets_application_config_file_template_fk` FOREIGN KEY (`deployment_template_id`) REFERENCES `assets_application_deployment_template` (`id`)
);

CREATE TABLE `assets_application_log_definition` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `name` varchar(128) NOT NULL, `path_pattern` varchar(512) NOT NULL,
  `deployment_template_id` bigint NOT NULL, `extra_fields` json NOT NULL, `processing_rule_id` bigint DEFAULT NULL,
  -- 模板级采集过滤默认值（2026-09-19）：两条引用各自独立，NULL = 该方向不用过滤。
  -- 服务级可覆盖/关闭（见 assets_application_service_log_setting 的两列），最终由渲染决定
  -- 是否往 Filebeat 的 input 里写 include_lines / exclude_lines。规则方向由
  -- monitor_log_collection_filter_rule.rule_type 声明，落槽时校验一致。
  `filter_include_rule_id` bigint DEFAULT NULL, `filter_exclude_rule_id` bigint DEFAULT NULL, PRIMARY KEY (`id`),
  UNIQUE KEY `unique_template_log_name` (`deployment_template_id`,`name`),
  CONSTRAINT `assets_application_log_definition_template_fk` FOREIGN KEY (`deployment_template_id`) REFERENCES `assets_application_deployment_template` (`id`)
);

CREATE TABLE `assets_application_control_action` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `action` varchar(16) NOT NULL, `command` longtext NOT NULL, `timeout_seconds` int unsigned NOT NULL, `success_exit_codes` json NOT NULL,
  `deployment_template_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE KEY `unique_template_control_action` (`deployment_template_id`,`action`),
  CONSTRAINT `assets_application_control_action_template_fk` FOREIGN KEY (`deployment_template_id`) REFERENCES `assets_application_deployment_template` (`id`)
);

CREATE TABLE `assets_docker_control_config` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `container_name` varchar(255) NOT NULL, `docker_host` varchar(255) NOT NULL, `expected_image` varchar(255) NOT NULL, `expected_image_tag` varchar(128) NOT NULL,
  `deployment_template_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE KEY `deployment_id` (`deployment_template_id`),
  CONSTRAINT `assets_docker_control_config_template_fk` FOREIGN KEY (`deployment_template_id`) REFERENCES `assets_application_deployment_template` (`id`)
);

CREATE TABLE `assets_docker_compose_control_config` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `project_name` varchar(255) NOT NULL, `service_name` varchar(255) NOT NULL, `compose_file_path` varchar(512) NOT NULL, `working_directory` varchar(512) NOT NULL,
  `env_file` varchar(512) NOT NULL, `expected_image` varchar(255) NOT NULL, `expected_image_tag` varchar(128) NOT NULL,
  `deployment_template_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE KEY `deployment_id` (`deployment_template_id`),
  CONSTRAINT `assets_docker_compose_control_config_template_fk` FOREIGN KEY (`deployment_template_id`) REFERENCES `assets_application_deployment_template` (`id`)
);

CREATE TABLE `assets_application_service` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `name` varchar(128) NOT NULL, `code` varchar(64) NOT NULL, `topology_type` varchar(16) NOT NULL, `access_address` varchar(255) NOT NULL,
  `enabled` BOOLEAN NOT NULL, `application_id` bigint NOT NULL, `cluster_profile_id` bigint DEFAULT NULL, `environment_id` bigint DEFAULT NULL,
  `application_version_id` bigint NOT NULL, `deployment_template_id` bigint NOT NULL, `business_system_id` bigint NOT NULL,
  `macro_values` json NOT NULL, `log_collection_enabled` BOOLEAN NOT NULL, `log_retention_tier_id` bigint DEFAULT NULL, PRIMARY KEY (`id`),
  UNIQUE KEY `unique_business_environment_service_code` (`business_system_id`,`environment_id`,`code`),
  UNIQUE KEY `unique_business_environment_service` (`business_system_id`,`environment_id`,`name`),
  CONSTRAINT `assets_application_service_application_fk` FOREIGN KEY (`application_id`) REFERENCES `assets_application` (`id`),
  CONSTRAINT `assets_application_service_version_fk` FOREIGN KEY (`application_version_id`) REFERENCES `assets_application_version` (`id`),
  CONSTRAINT `assets_application_service_template_fk` FOREIGN KEY (`deployment_template_id`) REFERENCES `assets_application_deployment_template` (`id`),
  CONSTRAINT `assets_application_service_system_fk` FOREIGN KEY (`business_system_id`) REFERENCES `assets_business_system` (`id`)
);

CREATE TABLE `assets_application_deployment` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `instance_name` varchar(128) NOT NULL, `enabled` BOOLEAN NOT NULL, `host_id` bigint NOT NULL, `last_status_check_time` datetime(6) DEFAULT NULL,
  `runtime_status` varchar(16) NOT NULL, `runtime_status_output` longtext NOT NULL, `ha_role` varchar(16) NOT NULL, `runtime_variables` json NOT NULL,
  PRIMARY KEY (`id`), UNIQUE KEY `unique_host_application_instance` (`host_id`,`instance_name`),
  CONSTRAINT `assets_application_deployment_host_fk` FOREIGN KEY (`host_id`) REFERENCES `assets_host` (`id`)
);

CREATE TABLE `assets_application_service_deployment` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `enabled` BOOLEAN NOT NULL, `deployment_id` bigint NOT NULL, `service_id` bigint NOT NULL, PRIMARY KEY (`id`),
  UNIQUE KEY `unique_application_service_deployment` (`service_id`,`deployment_id`),
  CONSTRAINT `assets_application_service_deployment_deployment_fk` FOREIGN KEY (`deployment_id`) REFERENCES `assets_application_deployment` (`id`),
  CONSTRAINT `assets_application_service_deployment_service_fk` FOREIGN KEY (`service_id`) REFERENCES `assets_application_service` (`id`)
);

CREATE TABLE `assets_application_service_log_setting` (
  `id` bigint NOT NULL AUTO_INCREMENT, `create_time` datetime(6) NOT NULL, `update_time` datetime(6) NOT NULL, `remark` longtext,
  `collection_enabled` BOOLEAN DEFAULT NULL, `log_definition_id` bigint NOT NULL, `retention_tier_id` bigint DEFAULT NULL,
  `service_id` bigint NOT NULL,
  -- 采集过滤的服务级覆盖（2026-09-19）：两列都是**三态** ——
  -- NULL = 继承模板的对应方向；0 = 显式关闭该方向（模板配了也不过滤）；>0 = 指定规则。
  -- 分成两列是因为一条日志可以同时有白名单与黑名单（Filebeat 上 include_lines 先、exclude_lines 后）。
  -- **这两列不能挂外键**：0 是有效取值，挂上外键就会被当成"指向 id=0 的规则"，保存即报
  -- 「关联资产不存在」（真库上 include 列的 Django 时代外键已由迁移 000040 摘掉；规则被删/停用
  -- 由渲染侧降级成"该方向不过滤 + 下发告警"）。守卫见
  -- internal/assets/log_filter_rule_fk_guard_test.go。
  `collection_filter_rule_id` bigint DEFAULT NULL, `collection_exclude_filter_rule_id` bigint DEFAULT NULL,
  `format_verified_at` datetime(6) DEFAULT NULL, `format_verified_fingerprint` varchar(64) NOT NULL DEFAULT '',
  `format_verified_source` varchar(16) NOT NULL DEFAULT '', `format_verified_by` varchar(150) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `unique_service_log_setting` (`service_id`,`log_definition_id`),
  CONSTRAINT `assets_application_service_log_setting_log_fk` FOREIGN KEY (`log_definition_id`) REFERENCES `assets_application_log_definition` (`id`),
  CONSTRAINT `assets_application_service_log_setting_service_fk` FOREIGN KEY (`service_id`) REFERENCES `assets_application_service` (`id`)
);
-- Snapshot source: `SHOW CREATE TABLE` against the live migration database for the host
-- system/hardware snapshot tables used by the typed host list queries.

CREATE TABLE `assets_hostsystem` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `os_type` varchar(64) DEFAULT NULL,
  `os_version` varchar(128) DEFAULT NULL,
  `kernel_version` varchar(128) DEFAULT NULL,
  `hostname` varchar(128) DEFAULT NULL,
  `agent_version` varchar(64) DEFAULT NULL,
  `host_id` bigint NOT NULL,
  `collected_at` datetime(6) DEFAULT NULL,
  `collector_source` varchar(32) DEFAULT NULL,
  `timezone_name` varchar(64) DEFAULT NULL,
  `utc_offset` varchar(16) DEFAULT NULL,
  `os_id` varchar(64) DEFAULT NULL,
  `os_id_like` varchar(128) DEFAULT NULL,
  `os_version_id` varchar(64) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `host_id` (`host_id`),
  CONSTRAINT `assets_hostsystem_host_id_94282b79_fk_assets_host_id` FOREIGN KEY (`host_id`) REFERENCES `assets_host` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=261 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `assets_hosthardware` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `cpu_cores` int DEFAULT NULL,
  `cpu_model` varchar(255) DEFAULT NULL,
  `memory_gb` double DEFAULT NULL,
  `disk_total_gb` double DEFAULT NULL,
  `architecture` varchar(64) DEFAULT NULL,
  `host_id` bigint NOT NULL,
  `collected_at` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `host_id` (`host_id`),
  CONSTRAINT `assets_hosthardware_host_id_b48623b8_fk_assets_host_id` FOREIGN KEY (`host_id`) REFERENCES `assets_host` (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=261 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE `agent_package` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `version` varchar(64) NOT NULL,
  `file` varchar(512) NOT NULL,
  `sha256` char(64) NOT NULL,
  `size_bytes` bigint NOT NULL,
  `is_active` BOOLEAN NOT NULL DEFAULT '0',
  `create_time` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`id`)
);

CREATE TABLE `assets_agent_job` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `job_id` varchar(128) NOT NULL,
  `instance_name` varchar(128) NOT NULL,
  `job_type` varchar(32) NOT NULL,
  `action` varchar(64) NOT NULL,
  `params` json NOT NULL,
  `timeout_seconds` int unsigned NOT NULL,
  `status` varchar(16) NOT NULL,
  `picked_at` datetime(6) DEFAULT NULL,
  `finished_at` datetime(6) DEFAULT NULL,
  `result_data` json NOT NULL,
  `error_message` longtext NOT NULL,
  `host_id` bigint DEFAULT NULL,
  `client_request_id` varchar(128) DEFAULT NULL,
  `exit_code` int NOT NULL,
  `stderr` longtext NOT NULL,
  `stdout` longtext NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `job_id` (`job_id`),
  UNIQUE KEY `client_request_id` (`client_request_id`),
  KEY `assets_agent_job_host_id_06d13414_fk_assets_host_id` (`host_id`),
  KEY `assets_agen_agent_i_ed2f13_idx` (`instance_name`,`status`),
  KEY `assets_agen_status_d7c3d4_idx` (`status`,`create_time`),
  CONSTRAINT `assets_agent_job_host_id_06d13414_fk_assets_host_id` FOREIGN KEY (`host_id`) REFERENCES `assets_host` (`id`),
  CONSTRAINT `assets_agent_job_chk_1` CHECK ((`timeout_seconds` >= 0))
);

CREATE TABLE `assets_cloudaccount` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `name` varchar(128) NOT NULL,
  `cloud_type` varchar(32) NOT NULL,
  `endpoint` varchar(255) DEFAULT NULL,
  `username` varchar(128) DEFAULT NULL,
  `password` varchar(128) DEFAULT NULL,
  `access_key` varchar(128) DEFAULT NULL,
  `secret_key` varchar(128) DEFAULT NULL,
  PRIMARY KEY (`id`)
);

CREATE TABLE `assets_hostdisk` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `device` varchar(64) NOT NULL,
  `mount_point` varchar(128) DEFAULT NULL,
  `size_gb` double DEFAULT NULL,
  `used_gb` double DEFAULT NULL,
  `filesystem` varchar(64) DEFAULT NULL,
  `host_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `assets_hostdisk_host_id_c1a29894_fk_assets_host_id` (`host_id`),
  CONSTRAINT `assets_hostdisk_host_id_c1a29894_fk_assets_host_id` FOREIGN KEY (`host_id`) REFERENCES `assets_host` (`id`)
);

CREATE TABLE `assets_hostruntime` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `create_time` datetime(6) NOT NULL,
  `update_time` datetime(6) NOT NULL,
  `remark` longtext,
  `cpu_usage_percent` double DEFAULT NULL,
  `cpu_times` json NOT NULL,
  `memory_usage_percent` double DEFAULT NULL,
  `memory` json NOT NULL,
  `disk_io` json NOT NULL,
  `os_uptime_seconds` bigint DEFAULT NULL,
  `os_boot_time` datetime(6) DEFAULT NULL,
  `metrics_sample_window_ms` int unsigned DEFAULT NULL,
  `static_fingerprint` varchar(64) NOT NULL,
  `collected_at` datetime(6) DEFAULT NULL,
  `host_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `host_id` (`host_id`),
  CONSTRAINT `assets_hostruntime_host_id_c8ce3dd4_fk_assets_host_id` FOREIGN KEY (`host_id`) REFERENCES `assets_host` (`id`),
  CONSTRAINT `assets_hostruntime_chk_1` CHECK ((`metrics_sample_window_ms` >= 0))
);

CREATE TABLE `assets_webssh_temp_credential` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `session_pk` int DEFAULT NULL,
  `created_at` datetime(6) NOT NULL,
  `credential_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `credential_id` (`credential_id`),
  KEY `assets_webssh_temp_credential_session_pk_70b0ba26` (`session_pk`),
  CONSTRAINT `assets_webssh_temp_c_credential_id_fc449ce0_fk_assets_cr` FOREIGN KEY (`credential_id`) REFERENCES `assets_credential` (`id`)
);
