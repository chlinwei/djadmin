#!/usr/bin/env bash
set -euo pipefail

INSTALL_PATH=/usr/local/bin/autoadmin
CONFIG_DIR=/etc/autoadmin
CONFIG_PATH="$CONFIG_DIR/config.env"
SERVICE_PATH=/etc/systemd/system/autoadmin.service
SERVICE_USER=autoadmin
BINARY_SOURCE=
CONFIG_SOURCE=
START_SERVICE=false

usage() {
  cat <<'EOF'
Usage: install.sh --binary PATH --config PATH [--start]

Installs the autoadmin binary, environment file, and systemd unit.
The service is enabled but is only started when --start is supplied.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY_SOURCE=${2:-}; shift 2 ;;
    --config) CONFIG_SOURCE=${2:-}; shift 2 ;;
    --start) START_SERVICE=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "install.sh must run as root" >&2
  exit 1
fi
if [[ -z "$BINARY_SOURCE" || -z "$CONFIG_SOURCE" ]]; then
  echo "binary and config are required" >&2
  usage >&2
  exit 2
fi
if [[ ! -x "$BINARY_SOURCE" ]]; then
  echo "autoadmin binary is missing or not executable: $BINARY_SOURCE" >&2
  exit 1
fi
if [[ ! -f "$CONFIG_SOURCE" ]]; then
  echo "configuration file not found: $CONFIG_SOURCE" >&2
  exit 1
fi

for required_key in MYSQL_DSN JWT_SECRET RABBITMQ_URL; do
  if ! grep -q "^${required_key}=." "$CONFIG_SOURCE"; then
    echo "configuration must define a non-empty ${required_key}" >&2
    exit 2
  fi
done

if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  useradd --system --home-dir /var/lib/autoadmin --shell /usr/sbin/nologin "$SERVICE_USER"
fi

install -d -m 0750 -o root -g "$SERVICE_USER" "$CONFIG_DIR"
install -m 0755 "$BINARY_SOURCE" "$INSTALL_PATH.new"
install -m 0640 -o root -g "$SERVICE_USER" "$CONFIG_SOURCE" "$CONFIG_PATH.new"
install -m 0644 "$(dirname "$0")/autoadmin.service" "$SERVICE_PATH.new"
mv -f "$INSTALL_PATH.new" "$INSTALL_PATH"
mv -f "$CONFIG_PATH.new" "$CONFIG_PATH"
mv -f "$SERVICE_PATH.new" "$SERVICE_PATH"

systemctl daemon-reload
systemctl enable autoadmin.service

if [[ "$START_SERVICE" == true ]]; then
  systemctl restart autoadmin.service
  systemctl --no-pager --quiet is-active autoadmin.service
  echo "autoadmin installed and running"
else
  echo "autoadmin installed but not started; run: systemctl start autoadmin.service"
fi