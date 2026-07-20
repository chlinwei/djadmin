from django.db import migrations

# dj-agent 通用 exporter 执行器在运行任意脚本前，会先注入以下 shell 变量（详见
# dj_agent/internal/executor/generic_exporter.go 的 buildExporterPreamble）：
# EXPORTER_NAME/PORT/VERSION/PACKAGE_BASE_URL/EXPECTED_SHA256/START_ARGS/
# DJ_AGENT_HOME/EXPORTER_HOME/PACKAGES_DIR/BIN_DIR/LOG_DIR/RUN_DIR/PID_FILE/BIN_PATH，
# 以下默认脚本据此编写，替代原先 Go 代码里硬编码的 node_exporter 安装/启停逻辑。

INSTALL_SCRIPT = '''ARCH=$(uname -m)
case "$ARCH" in
\tx86_64|amd64) PKG_ARCH="amd64" ;;
\taarch64|arm64) PKG_ARCH="arm64" ;;
\t*)
\t\techo "unsupported arch: $ARCH" >&2
\t\texit 1
\t\t;;
esac
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
TARBALL="node_exporter-${VERSION}.${OS}-${PKG_ARCH}.tar.gz"
TARBALL_PATH="${PACKAGES_DIR}/${TARBALL}"
EXTRACT_DIR="${PACKAGES_DIR}/node_exporter-${VERSION}.${OS}-${PKG_ARCH}"
GITHUB_URL="https://github.com/prometheus/node_exporter/releases/download/v${VERSION}/${TARBALL}"
LEGACY_TARBALL="node_exporter-${VERSION}-${OS}-${PKG_ARCH}.tar.gz"
LEGACY_URL="https://github.com/prometheus/node_exporter/releases/download/v${VERSION}/${LEGACY_TARBALL}"

if [ -n "$PACKAGE_BASE_URL" ]; then
\tPRIMARY_URL="${PACKAGE_BASE_URL}/${TARBALL}"
else
\tPRIMARY_URL="$GITHUB_URL"
fi

if [ ! -f "$TARBALL_PATH" ]; then
\trm -f "$TARBALL_PATH"
\techo "downloading node_exporter package ..." >&2
\tif command -v curl >/dev/null 2>&1; then
\t\tif ! curl -fsSL "$PRIMARY_URL" -o "$TARBALL_PATH"; then
\t\t\tcurl -fsSL "$GITHUB_URL" -o "$TARBALL_PATH" || curl -fsSL "$LEGACY_URL" -o "$TARBALL_PATH"
\t\tfi
\telif command -v wget >/dev/null 2>&1; then
\t\tif ! wget -q "$PRIMARY_URL" -O "$TARBALL_PATH"; then
\t\t\twget -q "$GITHUB_URL" -O "$TARBALL_PATH" || wget -q "$LEGACY_URL" -O "$TARBALL_PATH"
\t\tfi
\telse
\t\techo "curl/wget not found" >&2
\t\texit 1
\tfi
fi

if [ -n "$EXPECTED_SHA256" ]; then
\tif command -v sha256sum >/dev/null 2>&1; then
\t\tACTUAL_SHA256=$(sha256sum "$TARBALL_PATH" | awk '{print $1}')
\telif command -v shasum >/dev/null 2>&1; then
\t\tACTUAL_SHA256=$(shasum -a 256 "$TARBALL_PATH" | awk '{print $1}')
\telse
\t\tACTUAL_SHA256=""
\tfi
\tif [ -n "$ACTUAL_SHA256" ] && [ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]; then
\t\techo "sha256 mismatch: expected $EXPECTED_SHA256 got $ACTUAL_SHA256" >&2
\t\trm -f "$TARBALL_PATH"
\t\texit 1
\tfi
fi

if [ ! -d "$EXTRACT_DIR" ]; then
\tmkdir -p "$EXTRACT_DIR"
\ttar -xzf "$TARBALL_PATH" -C "$EXTRACT_DIR" --strip-components=1
fi

if [ ! -f "$EXTRACT_DIR/node_exporter" ]; then
\techo "node_exporter binary not found in package" >&2
\texit 1
fi

install -m 0755 "$EXTRACT_DIR/node_exporter" "$BIN_PATH"
echo "node_exporter installed at ${BIN_PATH}"
'''

START_SCRIPT = '''mkdir -p "$LOG_DIR" "$RUN_DIR"
if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
\techo "node_exporter already running pid=$(cat "$PID_FILE")"
\texit 0
fi
if command -v ss >/dev/null 2>&1 && ss -lnt 2>/dev/null | grep -q ":${PORT} "; then
\tif command -v pgrep >/dev/null 2>&1 && pgrep -f "${BIN_PATH}.*:${PORT}" >/dev/null 2>&1; then
\t\techo "node_exporter already running on port ${PORT}"
\t\texit 0
\tfi
fi
nohup "$BIN_PATH" --web.listen-address=":${PORT}" ${START_ARGS} >"${LOG_DIR}/node_exporter.out" 2>"${LOG_DIR}/node_exporter.err" < /dev/null &
echo $! > "$PID_FILE"
sleep 1
if ! kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
\techo "node_exporter failed to start" >&2
\texit 1
fi
echo "node_exporter started pid=$(cat "$PID_FILE")"
'''

STOP_SCRIPT = '''if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
\tkill "$(cat "$PID_FILE")" || true
\trm -f "$PID_FILE"
\techo "node_exporter stopped"
\texit 0
fi
if command -v pgrep >/dev/null 2>&1; then
\tPIDS=$(pgrep -f "${BIN_PATH}.*:${PORT}" || true)
\tif [ -n "$PIDS" ]; then
\t\tkill $PIDS || true
\t\trm -f "$PID_FILE"
\t\techo "node_exporter stopped by process match"
\t\texit 0
\tfi
fi
echo "node_exporter not running"
'''

STATUS_SCRIPT = '''if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
\techo "running pid=$(cat "$PID_FILE")"
\texit 0
fi
if command -v pgrep >/dev/null 2>&1 && pgrep -f "${BIN_PATH}.*:${PORT}" >/dev/null 2>&1; then
\techo "running (pid file missing)"
\texit 0
fi
echo "stopped"
exit 1
'''

UNINSTALL_SCRIPT = '''rm -f "$BIN_PATH" || true
find "$PACKAGES_DIR" -maxdepth 1 -type f -name 'node_exporter-*.tar.gz' -delete 2>/dev/null || true
find "$PACKAGES_DIR" -maxdepth 1 -type d -name 'node_exporter-*' -exec rm -rf {} + 2>/dev/null || true
if command -v ss >/dev/null 2>&1 && ss -lnt 2>/dev/null | grep -q ":${PORT} "; then
\techo "node_exporter port ${PORT} is still listening" >&2
\texit 1
fi
echo "node_exporter uninstalled"
'''


def seed_default_scripts(apps, schema_editor):
    """将原先硬编码在 dj-agent Go 代码里的 node_exporter 安装/卸载/启停/状态逻辑，
    迁移为 SoftwarePackage 的默认脚本文本（仅当字段为空时才回填，避免覆盖已有自定义内容）。"""
    SoftwarePackage = apps.get_model('monitor', 'SoftwarePackage')
    for pkg in SoftwarePackage.objects.filter(name='node_exporter'):
        update_fields = []
        if not pkg.install_script:
            pkg.install_script = INSTALL_SCRIPT
            update_fields.append('install_script')
        if not pkg.uninstall_script:
            pkg.uninstall_script = UNINSTALL_SCRIPT
            update_fields.append('uninstall_script')
        if not pkg.start_script:
            pkg.start_script = START_SCRIPT
            update_fields.append('start_script')
        if not pkg.stop_script:
            pkg.stop_script = STOP_SCRIPT
            update_fields.append('stop_script')
        if not pkg.status_script:
            pkg.status_script = STATUS_SCRIPT
            update_fields.append('status_script')
        if update_fields:
            pkg.save(update_fields=update_fields)


def noop_reverse(apps, schema_editor):
    pass


class Migration(migrations.Migration):

    dependencies = [
        ('monitor', '0007_monitortarget_env_vars_monitortarget_install_script_and_more'),
    ]

    operations = [
        migrations.RunPython(seed_default_scripts, noop_reverse),
    ]
