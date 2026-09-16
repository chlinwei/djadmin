-- 回滚主机身份改造：恢复 agent_id 列名与结构。
--
-- 注意：assets_host.agent_id 的**数据不可恢复**（删除时未备份，且删除后已无任何写入方），
-- 这里只恢复列结构与唯一索引，回滚后该列为全 NULL；快照列因只是改名回填，
-- 回滚同样只恢复列名，不还原 Django 时代的旧 agent_id 值。
--
-- PG 侧差异：CHANGE COLUMN 拆成 RENAME COLUMN（类型未变）；ADD UNIQUE KEY → ADD CONSTRAINT。

ALTER TABLE assets_agent_job RENAME COLUMN instance_name TO agent_id;

ALTER TABLE inspection_target_execution RENAME COLUMN instance_name_snapshot TO agent_id_snapshot;

ALTER TABLE security_scan_target RENAME COLUMN instance_name_snapshot TO agent_id;

ALTER TABLE assets_host ADD COLUMN agent_id varchar(128) DEFAULT NULL;
ALTER TABLE assets_host ADD CONSTRAINT agent_id UNIQUE (agent_id);
