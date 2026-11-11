ALTER TABLE assets_application_log_definition
  ADD COLUMN exclude_directories JSON NOT NULL DEFAULT (JSON_ARRAY());