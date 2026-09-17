-- 回滚 sys_agent_token.api_id → agent_id（仅恢复列名，数据原样保留）。
ALTER TABLE sys_agent_token RENAME COLUMN api_id TO agent_id;
