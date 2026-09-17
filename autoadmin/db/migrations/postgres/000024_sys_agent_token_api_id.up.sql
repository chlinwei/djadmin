-- sys_agent_token.agent_id 改名为 api_id（与 mysql 侧 000024 同版本号、同语义）。
--
-- 背景见 mysql 侧同名文件：该列不承载主机身份，按 bind_mode 是 'global' 保留字
-- 或用户自填的外部 API 令牌标识，改名消除误导。DB 层无唯一约束，改名不影响索引。
ALTER TABLE sys_agent_token RENAME COLUMN agent_id TO api_id;
