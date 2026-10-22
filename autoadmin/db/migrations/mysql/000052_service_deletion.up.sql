CREATE TABLE assets_service_deletion (
  service_id bigint NOT NULL PRIMARY KEY,
  project_code varchar(64) NOT NULL,
  business_system_code varchar(64) NOT NULL,
  environment_code varchar(64) NOT NULL,
  service_code varchar(64) NOT NULL
);
