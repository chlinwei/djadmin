-- sys_agent_token.agent_id 改名为 api_id。
--
-- 背景：该列不承载主机身份（主机身份已统一为 assets_host.instance_name，见 000020），
-- 按 bind_mode 有两种语义：'agent' 模式恒为保留字 'global'；'api' 模式是用户自己填的
-- 外部 API 令牌标识（前端标签一直叫 "Api ID"）。列名 agent_id 有误导性，改名为 api_id。
--
-- 该列 DB 层无唯一约束（唯一性由服务层 CountAPITokensByApiID 保证），改名不影响索引。
ALTER TABLE `sys_agent_token`
  CHANGE COLUMN `agent_id` `api_id` varchar(128) NOT NULL;
