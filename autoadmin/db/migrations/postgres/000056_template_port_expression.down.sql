-- Refuse lossy rollback if macro expressions or invalid ports remain.
ALTER TABLE assets_application_port ADD CONSTRAINT template_port_numeric_rollback
  CHECK (port ~ '^[1-9][0-9]{0,4}$' AND port::integer BETWEEN 1 AND 65535);
ALTER TABLE assets_application_port ALTER COLUMN port TYPE integer USING port::integer;
ALTER TABLE assets_application_port DROP CONSTRAINT template_port_numeric_rollback;
