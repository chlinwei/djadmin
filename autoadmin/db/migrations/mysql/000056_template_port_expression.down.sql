-- Refuse lossy rollback if macro expressions or invalid ports remain.
ALTER TABLE assets_application_port ADD CONSTRAINT template_port_numeric_rollback
  CHECK (port REGEXP '^[1-9][0-9]{0,4}$' AND CAST(port AS UNSIGNED) BETWEEN 1 AND 65535);
ALTER TABLE assets_application_port MODIFY COLUMN port int unsigned NOT NULL;
ALTER TABLE assets_application_port DROP CHECK template_port_numeric_rollback;
