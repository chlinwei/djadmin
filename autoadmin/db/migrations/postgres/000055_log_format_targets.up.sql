CREATE TABLE assets_log_format_target (
  service_id bigint NOT NULL,
  log_definition_id bigint NOT NULL,
  deployment_id bigint NOT NULL,
  fingerprint varchar(64) NOT NULL,
  status varchar(24) NOT NULL,
  attempts integer NOT NULL DEFAULT 0,
  next_attempt_at timestamp(6) NOT NULL,
  lease_token varchar(64) NOT NULL DEFAULT '',
  report jsonb NOT NULL,
  actor varchar(150) NOT NULL DEFAULT '',
  reason text NOT NULL,
  update_time timestamp(6) NOT NULL,
  PRIMARY KEY (service_id, log_definition_id, deployment_id),
  CONSTRAINT log_format_service_fk FOREIGN KEY (service_id) REFERENCES assets_application_service(id) ON DELETE CASCADE,
  CONSTRAINT log_format_definition_fk FOREIGN KEY (log_definition_id) REFERENCES assets_application_log_definition(id) ON DELETE CASCADE,
  CONSTRAINT log_format_deployment_fk FOREIGN KEY (deployment_id) REFERENCES assets_application_deployment(id) ON DELETE CASCADE
);
CREATE INDEX log_format_due_idx ON assets_log_format_target (status, next_attempt_at);
CREATE INDEX log_format_definition_idx ON assets_log_format_target (log_definition_id);
CREATE INDEX log_format_deployment_idx ON assets_log_format_target (deployment_id);