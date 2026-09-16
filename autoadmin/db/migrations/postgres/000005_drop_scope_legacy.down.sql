-- 不可无损回滚（legacy 列与数据已删）；回滚仅恢复列结构。
-- PG 侧差异：MySQL 的 json NOT NULL 在 PG 同样是 jsonb NOT NULL，且同样要求表内无数据
-- （PG 报 23502；原语句在空表下才成立），为与 mysql 侧语义一致不补默认值。
ALTER TABLE inspection_task
  ADD COLUMN logical_service_id bigint DEFAULT NULL,
  ADD COLUMN selected_host_ids jsonb NOT NULL;

ALTER TABLE inspection_group ADD COLUMN scope varchar(24) NOT NULL DEFAULT 'per_deployment';
