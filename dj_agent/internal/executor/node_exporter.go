package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

const defaultNodeExporterVersion = "1.8.2"
const defaultNodeExporterPort = 9100

// installNodeExporter 在本地安装并启动 Node Exporter
func (e *Executor) installNodeExporter(ctx context.Context, job protocol.Job) protocol.JobResult {
	port, err := resolveNodeExporterPort(job.Params)
	if err != nil {
		return builtinFailedResult(job, err)
	}

	version := resolveNodeExporterVersion(job.Params)
	script := fmt.Sprintf(`set -euo pipefail
PORT=%d
VERSION="%s"
DJ_AGENT_HOME="${HOME}/dj-agent"
PACKAGES_DIR="${DJ_AGENT_HOME}/packages"
BIN_DIR="${DJ_AGENT_HOME}/bin"
LOG_DIR="${DJ_AGENT_HOME}/logs"
RUN_DIR="${DJ_AGENT_HOME}/run"
PID_FILE="${RUN_DIR}/node_exporter.pid"
BIN_PATH="${BIN_DIR}/node_exporter"
ENV_FILE="${DJ_AGENT_HOME}/node_exporter.env"
START_SCRIPT="${BIN_DIR}/node_exporter_start.sh"
STOP_SCRIPT="${BIN_DIR}/node_exporter_stop.sh"
STATUS_SCRIPT="${BIN_DIR}/node_exporter_status.sh"
RESTART_SCRIPT="${BIN_DIR}/node_exporter_restart.sh"

mkdir -p "${PACKAGES_DIR}" "${BIN_DIR}" "${LOG_DIR}" "${RUN_DIR}"

ARCH=$(uname -m)
case "$ARCH" in
	x86_64|amd64) PKG_ARCH="amd64" ;;
	aarch64|arm64) PKG_ARCH="arm64" ;;
	*)
		echo "unsupported arch: $ARCH" >&2
		exit 1
		;;
esac

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
TARBALL="node_exporter-${VERSION}.${OS}-${PKG_ARCH}.tar.gz"
TARBALL_PATH="${PACKAGES_DIR}/${TARBALL}"
EXTRACT_DIR="${PACKAGES_DIR}/node_exporter-${VERSION}.${OS}-${PKG_ARCH}"
URL="https://github.com/prometheus/node_exporter/releases/download/v${VERSION}/${TARBALL}"
LEGACY_TARBALL="node_exporter-${VERSION}-${OS}-${PKG_ARCH}.tar.gz"
LEGACY_URL="https://github.com/prometheus/node_exporter/releases/download/v${VERSION}/${LEGACY_TARBALL}"

if [ ! -f "$TARBALL_PATH" ]; then
	rm -f "$TARBALL_PATH"
	echo "downloading node_exporter package ..." >&2
	echo "primary url: $URL" >&2
	if command -v curl >/dev/null 2>&1; then
		if ! curl -fsSL "$URL" -o "$TARBALL_PATH"; then
			echo "primary url failed, fallback url: $LEGACY_URL" >&2
			curl -fsSL "$LEGACY_URL" -o "$TARBALL_PATH"
		fi
	elif command -v wget >/dev/null 2>&1; then
		if ! wget -q "$URL" -O "$TARBALL_PATH"; then
			echo "primary url failed, fallback url: $LEGACY_URL" >&2
			wget -q "$LEGACY_URL" -O "$TARBALL_PATH"
		fi
	else
		echo "curl/wget not found" >&2
		exit 1
	fi
fi

if [ ! -d "$EXTRACT_DIR" ]; then
	mkdir -p "$EXTRACT_DIR"
	tar -xzf "$TARBALL_PATH" -C "$EXTRACT_DIR" --strip-components=1
fi

if [ ! -f "$EXTRACT_DIR/node_exporter" ]; then
	echo "node_exporter binary not found in package" >&2
	exit 1
fi

install -m 0755 "$EXTRACT_DIR/node_exporter" "$BIN_PATH"

cat > "$ENV_FILE" <<ENV
PORT=${PORT}
DJ_AGENT_HOME=${DJ_AGENT_HOME}
BIN_PATH=${BIN_PATH}
LOG_DIR=${LOG_DIR}
RUN_DIR=${RUN_DIR}
PID_FILE=${PID_FILE}
ENV

cat > "$START_SCRIPT" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
ENV_FILE="${HOME}/dj-agent/node_exporter.env"
[ -f "$ENV_FILE" ] || { echo "env file not found: $ENV_FILE"; exit 1; }
source "$ENV_FILE"
mkdir -p "$LOG_DIR" "$RUN_DIR"

if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
	echo "node_exporter already running pid=$(cat "$PID_FILE")"
	exit 0
fi

if command -v ss >/dev/null 2>&1 && ss -lnt 2>/dev/null | grep -q ":${PORT} "; then
	if command -v pgrep >/dev/null 2>&1 && pgrep -f "${BIN_PATH}.*:${PORT}" >/dev/null 2>&1; then
		echo "node_exporter already running on port ${PORT}"
		exit 0
	fi
fi

nohup "$BIN_PATH" --web.listen-address=":${PORT}" >"${LOG_DIR}/node_exporter.out" 2>"${LOG_DIR}/node_exporter.err" < /dev/null &
echo $! > "$PID_FILE"
sleep 1
if ! kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
	echo "node_exporter failed to start" >&2
	exit 1
fi
echo "node_exporter started pid=$(cat "$PID_FILE")"
SCRIPT

cat > "$STOP_SCRIPT" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
ENV_FILE="${HOME}/dj-agent/node_exporter.env"
[ -f "$ENV_FILE" ] || { echo "env file not found: $ENV_FILE"; exit 1; }
source "$ENV_FILE"

if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
	kill "$(cat "$PID_FILE")" || true
	rm -f "$PID_FILE"
	echo "node_exporter stopped"
	exit 0
fi

if command -v pgrep >/dev/null 2>&1; then
	PIDS=$(pgrep -f "${BIN_PATH}.*:${PORT}" || true)
	if [ -n "$PIDS" ]; then
		kill $PIDS || true
		rm -f "$PID_FILE"
		echo "node_exporter stopped by process match"
		exit 0
	fi
fi

echo "node_exporter not running"
SCRIPT

cat > "$STATUS_SCRIPT" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
ENV_FILE="${HOME}/dj-agent/node_exporter.env"
[ -f "$ENV_FILE" ] || { echo "env file not found: $ENV_FILE"; exit 1; }
source "$ENV_FILE"

if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
	echo "running pid=$(cat "$PID_FILE")"
	exit 0
fi

if command -v pgrep >/dev/null 2>&1 && pgrep -f "${BIN_PATH}.*:${PORT}" >/dev/null 2>&1; then
	echo "running (pid file missing)"
	exit 0
fi

echo "stopped"
exit 1
SCRIPT

cat > "$RESTART_SCRIPT" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
"${HOME}/dj-agent/bin/node_exporter_stop.sh" || true
"${HOME}/dj-agent/bin/node_exporter_start.sh"
SCRIPT

chmod +x "$START_SCRIPT" "$STOP_SCRIPT" "$STATUS_SCRIPT" "$RESTART_SCRIPT"

"$START_SCRIPT"

if command -v curl >/dev/null 2>&1; then
	curl -fsSL "http://127.0.0.1:${PORT}/metrics" >/dev/null
fi

echo "node_exporter installed under ${DJ_AGENT_HOME} and started on port ${PORT}"
`, port, version)

	return runBuiltinShell(ctx, job, script)
}

// uninstallNodeExporter 卸载并停止 Node Exporter
func (e *Executor) uninstallNodeExporter(ctx context.Context, job protocol.Job) protocol.JobResult {
	port, err := resolveNodeExporterPort(job.Params)
	if err != nil {
		return builtinFailedResult(job, err)
	}

	script := fmt.Sprintf(`set -euo pipefail
PORT=%d
DJ_AGENT_HOME="${HOME}/dj-agent"
PACKAGES_DIR="${DJ_AGENT_HOME}/packages"
BIN_DIR="${DJ_AGENT_HOME}/bin"
RUN_DIR="${DJ_AGENT_HOME}/run"
LOG_DIR="${DJ_AGENT_HOME}/logs"
ENV_FILE="${DJ_AGENT_HOME}/node_exporter.env"
PID_FILE="${RUN_DIR}/node_exporter.pid"
START_SCRIPT="${BIN_DIR}/node_exporter_start.sh"
STOP_SCRIPT="${BIN_DIR}/node_exporter_stop.sh"
STATUS_SCRIPT="${BIN_DIR}/node_exporter_status.sh"
RESTART_SCRIPT="${BIN_DIR}/node_exporter_restart.sh"
BIN_PATH="${BIN_DIR}/node_exporter"

if [ -x "$STOP_SCRIPT" ]; then
	"$STOP_SCRIPT" || true
fi

if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
	kill "$(cat "$PID_FILE")" || true
	rm -f "$PID_FILE" || true
fi

if command -v pgrep >/dev/null 2>&1; then
	PIDS=$(pgrep -f "${BIN_PATH}.*:${PORT}" || true)
	if [ -n "$PIDS" ]; then
		kill $PIDS || true
	fi
fi

rm -f "$BIN_PATH" "$START_SCRIPT" "$STOP_SCRIPT" "$STATUS_SCRIPT" "$RESTART_SCRIPT" "$ENV_FILE" || true
rm -f "${RUN_DIR}/node_exporter.pid" || true

find "$PACKAGES_DIR" -maxdepth 1 -type f -name 'node_exporter-*.tar.gz' -delete 2>/dev/null || true
find "$PACKAGES_DIR" -maxdepth 1 -type d -name 'node_exporter-*' -exec rm -rf {} + 2>/dev/null || true

if command -v ss >/dev/null 2>&1 && ss -lnt 2>/dev/null | grep -q ":${PORT} "; then
  echo "node_exporter port ${PORT} is still listening" >&2
  exit 1
fi

echo "node_exporter uninstalled from ${DJ_AGENT_HOME}"
`, port)

	return runBuiltinShell(ctx, job, script)
}

// resolveNodeExporterPort 从参数中解析 Node Exporter 端口
func resolveNodeExporterPort(params map[string]any) (int, error) {
	raw, ok := params["exporter_port"]
	if !ok || raw == nil {
		return defaultNodeExporterPort, nil
	}

	switch typed := raw.(type) {
	case int:
		if typed <= 0 || typed > 65535 {
			return 0, fmt.Errorf("invalid exporter_port: %d", typed)
		}
		return typed, nil
	case float64:
		value := int(typed)
		if value <= 0 || value > 65535 {
			return 0, fmt.Errorf("invalid exporter_port: %d", value)
		}
		return value, nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return defaultNodeExporterPort, nil
		}
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed <= 0 || parsed > 65535 {
			return 0, fmt.Errorf("invalid exporter_port: %s", typed)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("invalid exporter_port type: %T", raw)
	}
}

// resolveNodeExporterVersion 从参数中解析 Node Exporter 版本
func resolveNodeExporterVersion(params map[string]any) string {
	normalize := func(raw string) string {
		text := strings.TrimSpace(raw)
		if text == "" {
			return ""
		}
		return strings.TrimPrefix(text, "v")
	}

	if raw, ok := params["version"]; ok && raw != nil {
		text := normalize(fmt.Sprintf("%v", raw))
		if text != "" {
			return text
		}
	}

	fromEnv := normalize(os.Getenv("DJ_AGENT_NODE_EXPORTER_VERSION"))
	if fromEnv != "" {
		return fromEnv
	}

	return defaultNodeExporterVersion
}

// runBuiltinShell 执行内置 Shell 脚本
func runBuiltinShell(ctx context.Context, job protocol.Job, script string) protocol.JobResult {
	started := time.Now()
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", script)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	finished := time.Now()
	result := protocol.JobResult{
		JobID:      job.JobID,
		Type:       job.Type,
		Action:     job.Action,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		StartedAt:  started,
		FinishedAt: finished,
		CostMS:     finished.Sub(started).Milliseconds(),
		ExitCode:   readExitCode(err),
	}

	switch {
	case err == nil:
		result.Status = protocol.StatusSuccess
	case strings.Contains(ctx.Err().Error(), "deadline"):
		result.Status = protocol.StatusTimeout
		result.Error = ctx.Err().Error()
	case strings.Contains(ctx.Err().Error(), "cancel"):
		result.Status = protocol.StatusCanceled
		result.Error = ctx.Err().Error()
	default:
		result.Status = protocol.StatusFailed
		result.Error = err.Error()
	}

	return result
}

// builtinFailedResult 创建失败结果（用于前置验证错误）
func builtinFailedResult(job protocol.Job, runErr error) protocol.JobResult {
	now := time.Now()
	return protocol.JobResult{
		JobID:      job.JobID,
		Type:       job.Type,
		Action:     job.Action,
		Status:     protocol.StatusFailed,
		ExitCode:   1,
		StartedAt:  now,
		FinishedAt: now,
		CostMS:     0,
		Error:      runErr.Error(),
	}
}
