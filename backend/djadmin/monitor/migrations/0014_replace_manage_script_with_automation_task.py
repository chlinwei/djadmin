# 架构调整：安装/卸载彻底废弃 manage_script（dj-agent 自解释 shell 脚本）机制，改为完全复用
# “自动化任务”功能（automation.PlaybookTemplate + automation.AutomationTask）。
# - SoftwarePackage 新增 install_task/uninstall_task（各自绑定一个 AutomationTask，playbook
#   内容在“模板->Playbook模板”维护）以及 service_file_content（systemd unit 文件内容，
#   安装 playbook 通过 extra_vars 拿到后写入 /usr/lib/systemd/system/<name>.service）。
# - MonitorTarget 不再保留主机级脚本副本（manage_script/env_vars 字段整体删除），
#   改为新增 last_install_job_id，记录最近一次安装/卸载对应的 AutomationExecutionJob id，
#   供前端跳转查看真实执行日志。
# 本次为纯结构调整（不兼容旧数据），旧 manage_script/env_vars 内容随字段删除一并作废，
# 迁移后需在监控软件仓库为每个软件包重新配置 install_task/uninstall_task/service_file_content。

from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):

	dependencies = [
		('monitor', '0013_manage_script_drop_start_args_var'),
		('automation', '0043_alter_automationtask_execution_timeout_seconds'),
	]

	operations = [
		migrations.RemoveField(
			model_name='softwarepackage',
			name='manage_script',
		),
		migrations.RemoveField(
			model_name='softwarepackage',
			name='env_vars',
		),
		migrations.RemoveField(
			model_name='monitortarget',
			name='manage_script',
		),
		migrations.RemoveField(
			model_name='monitortarget',
			name='env_vars',
		),
		migrations.AddField(
			model_name='softwarepackage',
			name='install_task',
			field=models.ForeignKey(
				blank=True, null=True, on_delete=django.db.models.deletion.SET_NULL,
				related_name='+', to='automation.automationtask',
				help_text='安装该软件包使用的自动化任务（Playbook），内容在“模板->Playbook模板”中维护',
			),
		),
		migrations.AddField(
			model_name='softwarepackage',
			name='uninstall_task',
			field=models.ForeignKey(
				blank=True, null=True, on_delete=django.db.models.deletion.SET_NULL,
				related_name='+', to='automation.automationtask',
				help_text='卸载该软件包使用的自动化任务（Playbook）',
			),
		),
		migrations.AddField(
			model_name='softwarepackage',
			name='service_file_content',
			field=models.TextField(
				blank=True, default='',
				help_text='systemd unit 文件内容，安装时写入 /usr/lib/systemd/system/<name>.service',
			),
		),
		migrations.AddField(
			model_name='monitortarget',
			name='last_install_job_id',
			field=models.PositiveIntegerField(
				blank=True, null=True,
				help_text='最近一次安装/卸载对应的 AutomationExecutionJob id，用于跳转查看执行日志',
			),
		),
	]
