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
  `agent_id` varchar(128) DEFAULT NULL,
  `environment_id` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `agent_id` (`agent_id`),
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
  `name` varchar(128) NOT NULL, `path_pattern` varchar(512) NOT NULL, `collection_enabled` BOOLEAN NOT NULL,
  `deployment_template_id` bigint NOT NULL, `extra_fields` json NOT NULL, `processing_rule_id` bigint DEFAULT NULL, PRIMARY KEY (`id`),
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
  UNIQUE KEY `code` (`code`), UNIQUE KEY `unique_business_environment_service` (`business_system_id`,`environment_id`,`name`),
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
  `service_id` bigint NOT NULL, `processing_rule_id` bigint DEFAULT NULL, `collection_filter_rule_id` bigint DEFAULT NULL, PRIMARY KEY (`id`),
  UNIQUE KEY `unique_service_log_setting` (`service_id`,`log_definition_id`),
  CONSTRAINT `assets_application_service_log_setting_log_fk` FOREIGN KEY (`log_definition_id`) REFERENCES `assets_application_log_definition` (`id`),
  CONSTRAINT `assets_application_service_log_setting_service_fk` FOREIGN KEY (`service_id`) REFERENCES `assets_application_service` (`id`)
);