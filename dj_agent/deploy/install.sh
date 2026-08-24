#!/usr/bin/env bash
set -euo pipefail

INSTALL_PATH=/usr/local/bin/dj-agent
CONFIG_DIR=/etc/dj-agent
CONFIG_PATH="$CONFIG_DIR/config.env"
SERVICE_PATH=/usr/lib/systemd/system/dj-agent.service
LEGACY_SERVICE_PATH=/etc/systemd/system/dj-agent.service
BINARY_SOURCE=
AGENT_ID=
GRPC_ADDR=
RUN_USER=root

usage() {
  cat <<'EOF'
Usage: install.sh --binary PATH --agent-id ID --grpc-addr HOST:PORT [--run-user USER]
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY_SOURCE=${2:-}; shift 2 ;;
    --agent-id) AGENT_ID=${2:-}; shift 2 ;;
    --grpc-addr) GRPC_ADDR=${2:-}; shift 2 ;;
    --run-user) RUN_USER=${2:-}; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "install.sh must run as root" >&2
  exit 1
fi
if [[ -z "$BINARY_SOURCE" || -z "$AGENT_ID" || -z "$GRPC_ADDR" ]]; then
  echo "binary, agent-id and grpc-addr are required" >&2
  usage >&2
  exit 2
fi
if [[ ! -f "$BINARY_SOURCE" ]]; then
  echo "agent binary not found: $BINARY_SOURCE" >&2
  exit 1
fi
if [[ ! "$AGENT_ID" =~ ^[A-Za-z0-9._:-]+$ ]]; then
  echo "invalid agent id" >&2
  exit 2
fi
if [[ ! "$GRPC_ADDR" =~ ^[A-Za-z0-9._:-]+:[0-9]+$ ]]; then
  echo "invalid grpc address" >&2
  exit 2
fi
if ! id "$RUN_USER" >/dev/null 2>&1; then
  echo "run user not found: $RUN_USER" >&2
  exit 2
fi

install -d -m 0755 "$CONFIG_DIR"
install -m 0755 "$BINARY_SOURCE" "$INSTALL_PATH.new"

cat > "$CONFIG_PATH.new" <<EOF
DJ_AGENT_ID=$AGENT_ID
DJ_AGENT_GRPC_FILE_ADDR=$GRPC_ADDR
DJ_AGENT_LOG_LEVEL=info
EOF
chmod 0600 "$CONFIG_PATH.new"

cat > "$SERVICE_PATH.new" <<EOF
[Unit]
Description=dj-agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$RUN_USER
EnvironmentFile=$CONFIG_PATH
StandardOutput=journal
StandardError=journal
SyslogIdentifier=dj-agent
ExecStart=$INSTALL_PATH
RestartSec=5
Restart=on-failure
SuccessExitStatus=0 143 130
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
EOF
chmod 0644 "$SERVICE_PATH.new"

mv -f "$INSTALL_PATH.new" "$INSTALL_PATH"
mv -f "$CONFIG_PATH.new" "$CONFIG_PATH"
rm -f "$LEGACY_SERVICE_PATH"
mv -f "$SERVICE_PATH.new" "$SERVICE_PATH"

systemctl daemon-reload
systemctl enable dj-agent.service
systemctl restart dj-agent.service
systemctl --no-pager --quiet is-active dj-agent.service

echo "dj-agent installed and running: $AGENT_ID"
