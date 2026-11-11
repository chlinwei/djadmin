ALTER TABLE assets_application_log_definition
  ADD COLUMN exclude_directories JSONB NOT NULL DEFAULT '[]'::jsonb;