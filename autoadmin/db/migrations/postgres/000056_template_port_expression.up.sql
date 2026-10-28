ALTER TABLE assets_application_port ALTER COLUMN port TYPE varchar(64) USING port::text;
