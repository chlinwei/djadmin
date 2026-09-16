-- 修复库内 agent 安装模板的渲染变量名：dj_agent_id → dj_agent_instance_name。
--
-- 背景：主机身份从 agent_id 改为 instance_name 时，`000001_agent_playbook_template`
-- 里的模板内容也改成了 `dj_agent_instance_name`。但 `000001` 早已应用（该库
-- schema_migrations 已是 19），**修改一个已应用的迁移文件不会重新执行**，所以库里
-- 存的仍是旧模板。后果是两条链路同时失效：
--   - install（Ansible）：agent_install.go 传 `-e dj_agent_instance_name=...`，而旧模板
--     只认 `dj_agent_id`（默认 ""），其 assert `dj_agent_id | length > 0` 直接失败；
--   - update（gRPC 自更新）：renderAgentEnvTemplate 找不到 `{{ dj_agent_instance_name }}`，
--     渲染出的 config.env 里仍是 `DJ_AGENT_ID=...`，新版 agent 启动即报
--     "DJ_AGENT_INSTANCE_NAME is required" 并退出，表现为重启后不回连。
--
-- 因此这里用新迁移把库内模板原地改名（改的是模板本身，不是迁移文件）。
-- 三处替换与 `000001` 工作区版本的差异一一对应；REPLACE 幂等（替换后的
-- `dj_agent_instance_name` 不含 `dj_agent_id` 子串，重复执行不再变化）。
--
-- 注意：`automation_execution_job.template_content_snapshot` 里的历史快照**不在此处改写**，
-- 那是当时实际执行内容的审计记录，改写会破坏可追溯性。

UPDATE `automation_playbook_template`
SET `content` = REPLACE(
                  REPLACE(
                    REPLACE(`content`,
                      'DJ_AGENT_ID={{ dj_agent_id }}',
                      'DJ_AGENT_INSTANCE_NAME={{ dj_agent_instance_name }}'),
                    'dj_agent_id',
                    'dj_agent_instance_name'),
                  'dj-agent binary, agent_id and grpc_addr are required',
                  'dj-agent binary, instance_name and grpc_addr are required'),
    `update_time` = NOW(6)
WHERE `category` = 'agent';
