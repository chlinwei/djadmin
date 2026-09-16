-- 主机身份统一为 instance_name：彻底移除 agent_id 字段。
--
-- 背景：dj-agent 的全局标识一开始用独立的 agent_id 列承载，现改为直接用
-- assets_host.instance_name（agent 侧来源 DJ_AGENT_INSTANCE_NAME，握手按该值匹配主机行、
-- 网关会话也以该值为路由 key）。因此 agent_id 在业务层已无写入方，继续保留只会留下
-- 一份永远为空、却被多处读作路由 key 的字段。
--
-- 1) assets_host 丢弃 agent_id 列与唯一索引（业务唯一性由服务层按 instance_name / ip 校验）；
-- 2) 历史快照列改名为 instance_name_snapshot 并回填，使其与新列名语义一致；
-- 3) assets_agent_job.agent_id 改名为 instance_name（该列存的已是主机实例名）。

-- 1) assets_host：先删唯一索引再删列。
ALTER TABLE `assets_host` DROP INDEX `agent_id`;
ALTER TABLE `assets_host` DROP COLUMN `agent_id`;

-- 2a) 基线扫描目标快照：旧值是 Django 时代的 agent_id，与新列名不符，
--     按 host_id 用主机当前 instance_name 回填（host_name/host_ip 快照不受影响）。
ALTER TABLE `security_scan_target`
  CHANGE COLUMN `agent_id` `instance_name_snapshot` varchar(128) NOT NULL;

UPDATE `security_scan_target` t
JOIN `assets_host` h ON h.id = t.host_id
SET t.instance_name_snapshot = COALESCE(h.instance_name, '')
WHERE COALESCE(h.instance_name, '') <> '';

-- 2b) 巡检目标执行快照：同上。
ALTER TABLE `inspection_target_execution`
  CHANGE COLUMN `agent_id_snapshot` `instance_name_snapshot` varchar(128) NOT NULL;

UPDATE `inspection_target_execution` t
JOIN `assets_host` h ON h.id = t.host_id
SET t.instance_name_snapshot = COALESCE(h.instance_name, '')
WHERE t.host_id IS NOT NULL AND COALESCE(h.instance_name, '') <> '';

-- 3) agent 作业行：该列写入的已是主机实例名，仅名字滞后。
ALTER TABLE `assets_agent_job`
  CHANGE COLUMN `agent_id` `instance_name` varchar(128) NOT NULL;

UPDATE `assets_agent_job` j
JOIN `assets_host` h ON h.id = j.host_id
SET j.instance_name = COALESCE(h.instance_name, '')
WHERE j.host_id IS NOT NULL AND COALESCE(h.instance_name, '') <> '';
