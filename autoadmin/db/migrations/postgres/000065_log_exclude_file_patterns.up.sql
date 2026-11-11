ALTER TABLE assets_application_log_definition
  ADD COLUMN exclude_file_patterns JSONB NOT NULL DEFAULT '[]'::jsonb;
