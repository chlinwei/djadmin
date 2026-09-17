-- 回滚 sys_agent_token.api_id → agent_id（仅恢复列名，数据原样保留）。
ALTER TABLE `sys_agent_token`
  CHANGE COLUMN `api_id` `agent_id` varchar(128) NOT NULL;
