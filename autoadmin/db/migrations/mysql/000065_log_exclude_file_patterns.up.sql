ALTER TABLE assets_application_log_definition
  ADD COLUMN exclude_file_patterns JSON NOT NULL DEFAULT (JSON_ARRAY());
