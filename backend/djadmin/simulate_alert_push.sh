#!/usr/bin/env bash
# 手动模拟 Prometheus notifier 推送一条历史告警（firing -> resolved），
# 用于补录一次真实发生过、但因 HTTP SD 403 导致规则从未真正 fire、
# 从而没有走到 webhook 的压测告警。
#
# 用法：改好下面几个变量后执行 `bash simulate_alert_push.sh`
set -euo pipefail

# ===== 按实际情况修改 =====
BASE_URL="http://10.25.66.150:9000"                        # 部署 djadmin 的地址
TOKEN="spU3EpmgjdDgouwBnHVZPxptoiJqisoMPKIRzkPB5Yc"         # alert_webhook 明文 token（哈希后依然可用，无需改）
ALERTNAME="HighMemoryUsage"                                  # 改成规则文件里真实的 alertname
INSTANCE="192.168.1.x:9100"                                  # 改成被压测主机的 instance label（ip:port）
SEVERITY="critical"                                          # 改成规则里配置的 severity
STARTS_AT="2026-07-26T13:00:00Z"                             # 压测开始时间（UTC）
ENDS_AT="2026-07-26T13:15:00Z"                               # 压测结束/恢复时间（UTC），改成实际时间
# ==========================

URL="${BASE_URL}/monitor/alert-webhook/api/v2/alerts"

echo "=== 1) 推送 firing ==="
curl -sS -X POST "${URL}" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d @- <<EOF
[
  {
    "labels": {
      "alertname": "${ALERTNAME}",
      "severity": "${SEVERITY}",
      "instance": "${INSTANCE}"
    },
    "annotations": {
      "summary": "内存使用率过高（手动补录，实际压测未走到 webhook）"
    },
    "startsAt": "${STARTS_AT}",
    "endsAt": "0001-01-01T00:00:00Z",
    "generatorURL": "${BASE_URL}/graph"
  }
]
EOF
echo

echo "=== 2) 推送 resolved ==="
curl -sS -X POST "${URL}" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d @- <<EOF
[
  {
    "labels": {
      "alertname": "${ALERTNAME}",
      "severity": "${SEVERITY}",
      "instance": "${INSTANCE}"
    },
    "annotations": {
      "summary": "内存使用率过高（手动补录，实际压测未走到 webhook）"
    },
    "startsAt": "${STARTS_AT}",
    "endsAt": "${ENDS_AT}",
    "generatorURL": "${BASE_URL}/graph"
  }
]
EOF
echo
echo "完成。去 AlertHistory 列表页确认这条记录：alertname=${ALERTNAME}, instance=${INSTANCE}"
