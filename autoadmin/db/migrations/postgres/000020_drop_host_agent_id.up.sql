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
--
-- PG 侧差异：
--   1) MySQL 的 DROP INDEX（唯一索引）在 PG 是 DROP CONSTRAINT —— UNIQUE KEY 被翻译成
--      UNIQUE 约束（名字同为 agent_id，该名字在全库无冲突，与 db/schema/postgres 的命名约定一致）；
--   2) CHANGE COLUMN 旧名 新名 <类型> 拆成 PG 的两步；本次只改名、类型未变，故只用 RENAME COLUMN；
--   3) MySQL 多表 UPDATE ... JOIN ... SET → PG 的 UPDATE ... FROM（只回填非空 instance_name）。

-- 1) assets_host：先删唯一索引再删列。
ALTER TABLE assets_host DROP CONSTRAINT agent_id;
ALTER TABLE assets_host DROP COLUMN agent_id;

-- 2a) 基线扫描目标快照：旧值是 Django 时代的 agent_id，与新列名不符，
--     按 host_id 用主机当前 instance_name 回填（host_name/host_ip 快照不受影响）。
ALTER TABLE security_scan_target RENAME COLUMN agent_id TO instance_name_snapshot;

UPDATE security_scan_target t
SET instance_name_snapshot = COALESCE(h.instance_name, '')
FROM assets_host h
WHERE h.id = t.host_id AND COALESCE(h.instance_name, '') <> '';

-- 2b) 巡检目标执行快照：同上。
ALTER TABLE inspection_target_execution RENAME COLUMN agent_id_snapshot TO instance_name_snapshot;

UPDATE inspection_target_execution t
SET instance_name_snapshot = COALESCE(h.instance_name, '')
FROM assets_host h
WHERE h.id = t.host_id AND t.host_id IS NOT NULL AND COALESCE(h.instance_name, '') <> '';

-- 3) agent 作业行：该列写入的已是主机实例名，仅名字滞后。
ALTER TABLE assets_agent_job RENAME COLUMN agent_id TO instance_name;

UPDATE assets_agent_job j
SET instance_name = COALESCE(h.instance_name, '')
FROM assets_host h
WHERE h.id = j.host_id AND j.host_id IS NOT NULL AND COALESCE(h.instance_name, '') <> '';
