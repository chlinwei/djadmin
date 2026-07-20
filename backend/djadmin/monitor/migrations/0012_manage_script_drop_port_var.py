# 数据迁移：修复 node_exporter 默认 manage_script 里残留的 ${PORT} 变量引用。
# 背景：exporter_port 功能已按用户要求整体移除（dj-agent 不再注入 PORT 环境变量，见
# dj_agent/internal/executor/generic_exporter.go），但 0008/0009 迁移预置的
# node_exporter 默认 manage_script（start/stop/status/uninstall 动作）仍引用 ${PORT}，
# 导致脚本在 dj-agent 端以 `set -u` 执行时报 "PORT: unbound variable" 并以 exit 1 失败，
# 具体表现为：install 动作成功（不依赖 PORT），但 start 动作必然失败，
# 最终 MonitorTarget.install_status 停留在 failed。
#
# 修复方式：把脚本里所有 ${PORT} 引用替换为 node_exporter 自身的默认监听端口 9100
# （即不加 --web.listen-address 参数时的官方默认值，同时也是 start_args 字段
# help_text 里一直写明的默认端口）。仅对仍包含旧版 ${PORT} 特征片段的脚本做替换，
# 若脚本不含该特征（说明用户已自行改写过或本就不依赖 PORT），则完全不改动。

import re

from django.db import migrations

PORT_VAR_RE = re.compile(r'\$\{PORT\}')


def _drop_port_var(script):
	if '${PORT}' not in script:
		return script  # 不含旧版 PORT 变量引用：无需改动

	# node_exporter 默认监听端口为 9100（--web.listen-address 默认值），
	# 移除 exporter_port 功能后不再有任何变量能覆盖这个值，直接硬编码替换。
	return PORT_VAR_RE.sub('9100', script)


def drop_port_var(apps, schema_editor):
	for model_name in ('SoftwarePackage', 'MonitorTarget'):
		Model = apps.get_model('monitor', model_name)
		for obj in Model.objects.all():
			script = str(obj.manage_script or '')
			new_script = _drop_port_var(script)
			if new_script != script:
				obj.manage_script = new_script
				obj.save(update_fields=['manage_script'])


def noop_reverse(apps, schema_editor):
	pass


class Migration(migrations.Migration):

	dependencies = [
		('monitor', '0011_remove_monitortarget_exporter_port'),
	]

	operations = [
		migrations.RunPython(drop_port_var, noop_reverse),
	]
