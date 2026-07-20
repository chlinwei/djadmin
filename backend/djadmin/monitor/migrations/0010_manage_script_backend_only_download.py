# 数据迁移：去掉 manage_script 里残留的“直连 GitHub 官方源”下载兜底逻辑。
# 背景：0008 迁移预置的默认 node_exporter 安装脚本里，当 PACKAGE_BASE_URL（backend 本地
# 软件仓库路径）为空或从该路径下载失败时，会自动回退到直接访问 GitHub release 地址下载，
# 这意味着 dj-agent 有可能绕过 backend 直连外网，不满足“exporter 安装包只能通过 backend
# 下载”的要求（尤其是内网/离线环境下的 agent 本就无法访问外网，回退逻辑毫无意义还掩盖了
# “本地仓库未同步安装包”这一真正问题）。
#
# 这里只对仍包含旧版特征片段（GITHUB_URL=.../LEGACY_URL=...）的脚本做“最小侵入”的正则替换：
# - 删除 GITHUB_URL/LEGACY_TARBALL/LEGACY_URL 这三行变量定义；
# - 把“PACKAGE_BASE_URL 为空则回退 GITHUB_URL”的判断，改成“为空直接报错退出”；
# - 把 curl/wget 下载失败后的官方源兜底调用，简化为只请求 PACKAGE_BASE_URL 对应的地址。
# 若脚本不包含这些特征片段（说明用户已自行改写过），则完全不改动，避免覆盖用户的自定义内容。

import re

from django.db import migrations

GITHUB_URL_LINE_RE = re.compile(
	r'^[ \t]*GITHUB_URL="https://github\.com/prometheus/node_exporter/releases/download/v\$\{VERSION\}/\$\{TARBALL\}"\n',
	re.MULTILINE,
)
LEGACY_TARBALL_LINE_RE = re.compile(
	r'^[ \t]*LEGACY_TARBALL="node_exporter-\$\{VERSION\}-\$\{OS\}-\$\{PKG_ARCH\}\.tar\.gz"\n',
	re.MULTILINE,
)
LEGACY_URL_LINE_RE = re.compile(
	r'^[ \t]*LEGACY_URL="https://github\.com/prometheus/node_exporter/releases/download/v\$\{VERSION\}/\$\{LEGACY_TARBALL\}"\n',
	re.MULTILINE,
)
PRIMARY_URL_BLOCK_RE = re.compile(
	r'^([ \t]*)if \[ -n "\$PACKAGE_BASE_URL" \]; then\n'
	r'[ \t]*PRIMARY_URL="\$\{PACKAGE_BASE_URL\}/\$\{TARBALL\}"\n'
	r'[ \t]*else\n'
	r'[ \t]*PRIMARY_URL="\$GITHUB_URL"\n'
	r'[ \t]*fi\n',
	re.MULTILINE,
)
CURL_FALLBACK_RE = re.compile(
	r'^([ \t]*)if ! curl -fsSL "\$PRIMARY_URL" -o "\$TARBALL_PATH"; then\n'
	r'[ \t]*curl -fsSL "\$GITHUB_URL" -o "\$TARBALL_PATH" \|\| curl -fsSL "\$LEGACY_URL" -o "\$TARBALL_PATH"\n'
	r'[ \t]*fi\n',
	re.MULTILINE,
)
WGET_FALLBACK_RE = re.compile(
	r'^([ \t]*)if ! wget -q "\$PRIMARY_URL" -O "\$TARBALL_PATH"; then\n'
	r'[ \t]*wget -q "\$GITHUB_URL" -O "\$TARBALL_PATH" \|\| wget -q "\$LEGACY_URL" -O "\$TARBALL_PATH"\n'
	r'[ \t]*fi\n',
	re.MULTILINE,
)


def _strip_github_fallback(script):
	if 'GITHUB_URL=' not in script or 'LEGACY_URL=' not in script:
		return script  # 不含旧版特征片段：用户已自行改写过，不做任何改动

	text = script
	text = GITHUB_URL_LINE_RE.sub('', text)
	text = LEGACY_TARBALL_LINE_RE.sub('', text)
	text = LEGACY_URL_LINE_RE.sub('', text)

	def _primary_url_replacement(match):
		indent = match.group(1)
		return (
			f'{indent}if [ -z "$PACKAGE_BASE_URL" ]; then\n'
			f'{indent}\techo "PACKAGE_BASE_URL is empty: dj-agent only allows downloading exporter packages via backend, refusing to fall back to external source" >&2\n'
			f'{indent}\texit 1\n'
			f'{indent}fi\n'
			f'{indent}PRIMARY_URL="${{PACKAGE_BASE_URL}}/${{TARBALL}}"\n'
		)

	text = PRIMARY_URL_BLOCK_RE.sub(_primary_url_replacement, text)
	text = CURL_FALLBACK_RE.sub(lambda m: f'{m.group(1)}curl -fsSL "$PRIMARY_URL" -o "$TARBALL_PATH"\n', text)
	text = WGET_FALLBACK_RE.sub(lambda m: f'{m.group(1)}wget -q "$PRIMARY_URL" -O "$TARBALL_PATH"\n', text)
	return text


def strip_backend_only_download(apps, schema_editor):
	for model_name in ('SoftwarePackage', 'MonitorTarget'):
		Model = apps.get_model('monitor', model_name)
		for obj in Model.objects.all():
			script = str(obj.manage_script or '')
			new_script = _strip_github_fallback(script)
			if new_script != script:
				obj.manage_script = new_script
				obj.save(update_fields=['manage_script'])


def noop_reverse(apps, schema_editor):
	pass


class Migration(migrations.Migration):

	dependencies = [
		('monitor', '0009_merge_scripts_into_manage_script'),
	]

	operations = [
		migrations.RunPython(strip_backend_only_download, noop_reverse),
	]
