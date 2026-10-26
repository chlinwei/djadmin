CREATE TABLE `monitor_shared_log_source` (
  `id` bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `name` varchar(128) NOT NULL,
  `host_id` bigint NOT NULL,
  `business_system_id` bigint NOT NULL,
  `environment_id` bigint NOT NULL,
  `path_pattern` varchar(1024) NOT NULL,
  `processing_rule_id` bigint NOT NULL,
  `retention_tier_id` bigint NOT NULL,
  `include_pattern` text NOT NULL,
  `exclude_pattern` text NOT NULL,
  `generation` bigint NOT NULL DEFAULT 1,
  `revision` bigint NOT NULL DEFAULT 1,
  `archived` BOOLEAN NOT NULL DEFAULT FALSE,
  UNIQUE KEY shared_log_source_name (name),
  KEY shared_log_source_host (host_id),
  CONSTRAINT shared_log_source_host_fk FOREIGN KEY (host_id) REFERENCES assets_host(id),
  CONSTRAINT shared_log_source_business_fk FOREIGN KEY (business_system_id) REFERENCES assets_business_system(id),
  CONSTRAINT shared_log_source_environment_fk FOREIGN KEY (environment_id) REFERENCES assets_business_environment(id),
  CONSTRAINT shared_log_source_rule_fk FOREIGN KEY (processing_rule_id) REFERENCES monitor_log_processing_rule(id),
  CONSTRAINT shared_log_source_tier_fk FOREIGN KEY (retention_tier_id) REFERENCES monitor_log_retention_tier(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
CREATE TABLE `monitor_shared_log_source_service` (
  `source_id` bigint NOT NULL,
  `service_id` bigint NOT NULL,
  PRIMARY KEY (source_id, service_id),
  KEY shared_log_source_service_lookup (service_id, source_id),
  CONSTRAINT shared_log_source_service_source_fk FOREIGN KEY (source_id) REFERENCES monitor_shared_log_source(id),
  CONSTRAINT shared_log_source_service_service_fk FOREIGN KEY (service_id) REFERENCES assets_application_service(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
CREATE TABLE `monitor_shared_log_source_revocation` (
  `source_id` bigint NOT NULL,
  `generation` bigint NOT NULL,
  `stopped` BOOLEAN NOT NULL DEFAULT FALSE,
  `last_error` text NOT NULL,
  PRIMARY KEY (source_id, generation),
  KEY shared_log_source_pending_stop (stopped, source_id),
  CONSTRAINT shared_log_source_revocation_fk FOREIGN KEY (source_id) REFERENCES monitor_shared_log_source(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
