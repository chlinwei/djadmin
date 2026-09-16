-- 回滚 agent 安装模板的变量名（instance_name → agent_id），与 000021 互为逆操作。
-- 同样只改模板本身，不改写 automation_execution_job 的历史执行快照。

UPDATE `automation_playbook_template`
SET `content` = REPLACE(
                  REPLACE(
                    REPLACE(`content`,
                      'DJ_AGENT_INSTANCE_NAME={{ dj_agent_instance_name }}',
                      'DJ_AGENT_ID={{ dj_agent_id }}'),
                    'dj_agent_instance_name',
                    'dj_agent_id'),
                  'dj-agent binary, instance_name and grpc_addr are required',
                  'dj-agent binary, agent_id and grpc_addr are required'),
    `update_time` = NOW(6)
WHERE `category` = 'agent';
