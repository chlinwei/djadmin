-- Shell 类 playbook 模板：content_format 区分内容形态（playbook=YAML 全文 / shell=裸 bash 脚本）。
-- 执行链在渲染快照时对 shell 格式包装成单任务 playbook（见 internal/automation 的包装渲染）；
-- 校验侧 shell 格式走 shellcheck（上传或服务器 PATH），不再走 YAML 结构校验。
ALTER TABLE automation_playbook_template
  ADD COLUMN content_format varchar(16) NOT NULL DEFAULT 'playbook';

-- 作业快照同步记录模板形态：改模板形态不影响在途作业（与 template_content_snapshot 同语义）。
ALTER TABLE automation_execution_job
  ADD COLUMN template_content_format_snapshot varchar(16) NOT NULL DEFAULT 'playbook';
