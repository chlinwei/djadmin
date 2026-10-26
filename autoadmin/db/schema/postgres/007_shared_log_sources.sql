CREATE TABLE monitor_shared_log_source (
  id bigserial PRIMARY KEY,
  name varchar(128) NOT NULL UNIQUE,
  host_id bigint NOT NULL REFERENCES assets_host(id),
  business_system_id bigint NOT NULL REFERENCES assets_business_system(id),
  environment_id bigint NOT NULL REFERENCES assets_business_environment(id),
  path_pattern varchar(1024) NOT NULL,
  processing_rule_id bigint NOT NULL REFERENCES monitor_log_processing_rule(id),
  retention_tier_id bigint NOT NULL REFERENCES monitor_log_retention_tier(id),
  include_pattern text NOT NULL,
  exclude_pattern text NOT NULL,
  generation bigint NOT NULL DEFAULT 1,
  revision bigint NOT NULL DEFAULT 1,
  archived boolean NOT NULL DEFAULT FALSE
);
CREATE INDEX shared_log_source_host ON monitor_shared_log_source(host_id);
CREATE TABLE monitor_shared_log_source_service (
  source_id bigint NOT NULL REFERENCES monitor_shared_log_source(id),
  service_id bigint NOT NULL REFERENCES assets_application_service(id),
  PRIMARY KEY (source_id, service_id)
);
CREATE INDEX shared_log_source_service_lookup ON monitor_shared_log_source_service(service_id, source_id);
CREATE TABLE monitor_shared_log_source_revocation (
  source_id bigint NOT NULL REFERENCES monitor_shared_log_source(id),
  generation bigint NOT NULL,
  stopped boolean NOT NULL DEFAULT FALSE,
  last_error text NOT NULL,
  PRIMARY KEY (source_id, generation)
);
CREATE INDEX shared_log_source_pending_stop ON monitor_shared_log_source_revocation(stopped, source_id);
