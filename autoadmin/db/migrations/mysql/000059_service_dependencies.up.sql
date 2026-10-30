ALTER TABLE assets_business_system ADD CONSTRAINT assets_business_project_identity UNIQUE (id,project_id);
ALTER TABLE assets_application_service ADD CONSTRAINT assets_service_dependency_identity UNIQUE (id,business_system_id,environment_id);

CREATE TABLE assets_service_dependency_type (
  id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
  create_time datetime(6) NOT NULL,
  update_time datetime(6) NOT NULL,
  code varchar(64) NOT NULL,
  name varchar(128) NOT NULL,
  enabled boolean NOT NULL,
  description varchar(500) NOT NULL,
  CONSTRAINT assets_dependency_type_code_uniq UNIQUE (code),
  CONSTRAINT assets_dependency_type_name_uniq UNIQUE (name)
);
CREATE TABLE assets_service_dependency (
  id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
  create_time datetime(6) NOT NULL,
  update_time datetime(6) NOT NULL,
  source_service_id bigint NOT NULL,
  target_service_id bigint NOT NULL,
  source_business_id bigint NOT NULL,
  target_business_id bigint NOT NULL,
  project_id bigint NOT NULL,
  environment_id bigint NOT NULL,
  interaction_type_id bigint NOT NULL,
  strength varchar(16) NOT NULL DEFAULT 'unknown',
  protocol varchar(128) NOT NULL,
  function_scope varchar(120) NOT NULL,
  failure_impact varchar(500) NOT NULL,
  fallback_plan varchar(500) NOT NULL,
  CONSTRAINT assets_dependency_scope_uniq UNIQUE (source_service_id,target_service_id,interaction_type_id,function_scope),
  CONSTRAINT assets_dependency_no_self CHECK (source_service_id <> target_service_id),
  CONSTRAINT assets_dependency_strength CHECK (strength IN ('strong','weak','unknown')),
  CONSTRAINT assets_dependency_source_fk FOREIGN KEY (source_service_id,source_business_id,environment_id)
    REFERENCES assets_application_service (id,business_system_id,environment_id),
  CONSTRAINT assets_dependency_target_fk FOREIGN KEY (target_service_id,target_business_id,environment_id)
    REFERENCES assets_application_service (id,business_system_id,environment_id),
  CONSTRAINT assets_dependency_source_project_fk FOREIGN KEY (source_business_id,project_id)
    REFERENCES assets_business_system (id,project_id),
  CONSTRAINT assets_dependency_target_project_fk FOREIGN KEY (target_business_id,project_id)
    REFERENCES assets_business_system (id,project_id),
  CONSTRAINT assets_dependency_project_fk FOREIGN KEY (project_id) REFERENCES assets_project (id),
  CONSTRAINT assets_dependency_environment_fk FOREIGN KEY (environment_id) REFERENCES assets_business_environment (id),
  CONSTRAINT assets_dependency_type_fk FOREIGN KEY (interaction_type_id) REFERENCES assets_service_dependency_type (id),
  KEY assets_dependency_project_environment_idx (project_id,environment_id,id),
  KEY assets_dependency_target_idx (target_service_id,id)
);
INSERT INTO assets_service_dependency_type (create_time,update_time,code,name,enabled,description) VALUES
(UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'http','服务调用',TRUE,'HTTP、gRPC 等服务接口调用'),
(UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'database','数据库访问',TRUE,'关系型或其他数据库访问'),
(UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'cache','缓存访问',TRUE,'缓存读取与写入'),
(UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'publish','消息发布',TRUE,'向消息系统发布消息'),
(UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'consume','消息消费',TRUE,'从消息系统消费消息');
