-- 回滚主机身份改造：恢复 agent_id 列名与结构。
--
-- 注意：assets_host.agent_id 的**数据不可恢复**（删除时未备份，且删除后已无任何写入方），
-- 这里只恢复列结构与唯一索引，回滚后该列为全 NULL；快照列因只是改名回填，
-- 回滚同样只恢复列名，不还原 Django 时代的旧 agent_id 值。

ALTER TABLE `assets_agent_job`
  CHANGE COLUMN `instance_name` `agent_id` varchar(128) NOT NULL;

ALTER TABLE `inspection_target_execution`
  CHANGE COLUMN `instance_name_snapshot` `agent_id_snapshot` varchar(128) NOT NULL;

ALTER TABLE `security_scan_target`
  CHANGE COLUMN `instance_name_snapshot` `agent_id` varchar(128) NOT NULL;

ALTER TABLE `assets_host` ADD COLUMN `agent_id` varchar(128) DEFAULT NULL;
ALTER TABLE `assets_host` ADD UNIQUE KEY `agent_id` (`agent_id`);
