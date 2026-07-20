# 数据迁移：修复 node_exporter 默认 manage_script 里残留的 ${START_ARGS} 变量引用。
# 背景：与 0012 迁移修复的 ${PORT} 问题同源——SoftwarePackage/MonitorTarget 曾有独立的
# start_args 字段，由 dj-agent 在运行 manage_script 前注入同名 shell 变量 START_ARGS；
# 该字段和注入逻辑已被移除（见 dj_agent/internal/executor/generic_exporter.go 的
# buildExporterPreamble，现在仅注入 EXPORTER_NAME/VERSION/PACKAGE_BASE_URL/EXPECTED_SHA256
# 以及 env_vars 自定义变量），但 0008 迁移预置的默认 start 脚本仍引用 ${START_ARGS}，
# 导致 start 动作在 `set -euo pipefail` 下报 "START_ARGS: unbound variable" 并失败
# （install 动作不依赖该变量，所以能看到"文件已装好但仍失败"的现象）。
#
# 修复方式：如需自定义启动参数，现在应通过 env_vars 字段自行导出变量并在脚本内引用，
# 因此这里直接从脚本里删掉 "${START_ARGS} "（含其后紧跟的一个空格），不做变量替换。
# 仅对仍包含该特征片段的脚本做处理，未受影响的脚本（不含 ${START_ARGS}）保持不变。

from django.db import migrations

STALE_FRAGMENT = '${START_ARGS} '


def drop_start_args_var(apps, schema_editor):
	for model_name in ('SoftwarePackage', 'MonitorTarget'):
		Model = apps.get_model('monitor', model_name)
		for obj in Model.objects.all():
			script = str(obj.manage_script or '')
			if STALE_FRAGMENT not in script:
				continue
			new_script = script.replace(STALE_FRAGMENT, '')
			obj.manage_script = new_script
			obj.save(update_fields=['manage_script'])


def noop_reverse(apps, schema_editor):
	pass


class Migration(migrations.Migration):

	dependencies = [
		('monitor', '0012_manage_script_drop_port_var'),
	]

	operations = [
		migrations.RunPython(drop_start_args_var, noop_reverse),
	]
