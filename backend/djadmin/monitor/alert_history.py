"""Prometheus 告警历史：backend 替代 Alertmanager 的核心处理逻辑。

拆成独立模块（而不是塞进 views.py/tasks.py），是因为 webhook 接收（实时写入）和每日对账
（兜底订正）两条链路都要用同一套 fingerprint 算法与状态机规则，避免两处实现漂移导致
同一条告警被判定成不同的身份。
"""
from __future__ import annotations

import hashlib
from typing import Any

from django.db import transaction
from django.utils import timezone
from django.utils.dateparse import parse_datetime

from .models import AlertHistory, AlertNotificationEvent
from .prometheus_api import api_get

# Prometheus/Go 里 time.Time 零值的 RFC3339 表示。注意：这只是 Alertmanager 自己对外
# webhook 模板的约定（firing 时 endsAt 传零值）。Prometheus 内置 notifier 直连
# “Alertmanager API”（本项目这种 alerting.alertmanagers 配置）完全不是这个约定——
# firing 状态的 endsAt 实际是一个滚动的“未来兜底过期时间”（activeAt + resend_delay 窗口，
# 用于 Prometheus 挂掉后告警能自动过期），只有真正 resolved 时 endsAt 才会是不晚于当前时刻
# 的真实恢复时间。因此不能用“零值=firing/非零=resolved”判断，必须比较 endsAt 与当前时间
# 的先后关系：未来（或零值）→ 仍在 firing；不晚于当前时刻 → 已恢复。
GO_ZERO_TIME = '0001-01-01T00:00:00Z'


def _enqueue_notification(alert: AlertHistory, event_type: str) -> bool:
    event, created = AlertNotificationEvent.objects.get_or_create(
        deduplication_key=f'{alert.id}:{event_type}',
        defaults={
            'alert_id': alert.id,
            'event_type': event_type,
        },
    )
    if not created:
        return False

    def dispatch():
        from .tasks import send_alert_notification

        send_alert_notification.delay(event.id)

    transaction.on_commit(dispatch)
    return True


def compute_alert_fingerprint(labels: dict[str, Any] | None) -> str:
    """按排序后的 labels 计算稳定哈希，作为同一条告警跨多次推送/查询的身份标识。

    Prometheus notifier 推送给“Alertmanager”的 payload 本身不带 fingerprint 字段
    （那是 Alertmanager 自己对外查询接口才会算出来的），/api/v1/alerts 返回的原始数据
    同样没有，所以这里必须自己算，且 webhook 接收与对账任务必须用完全相同的算法。
    """
    items = sorted((str(k), str(v)) for k, v in (labels or {}).items())
    raw = '|'.join(f'{k}={v}' for k, v in items)
    return hashlib.sha1(raw.encode('utf-8')).hexdigest()


def _parse_rfc3339(value: str | None):
    text = str(value or '').strip()
    if not text or text == GO_ZERO_TIME:
        return None
    dt = parse_datetime(text)
    if dt is not None and timezone.is_naive(dt):
        dt = timezone.make_aware(dt, timezone.utc)
    return dt


def _resolve_ends_at(value: str | None, now):
    """判断这条推送代表“已恢复”还是“仍在 firing”，并返回 (is_resolved, ends_at_dt)。

    见模块顶部 GO_ZERO_TIME 注释：Prometheus 内置 notifier 发的 endsAt 对 firing 告警
    是“未来的兜底过期时间”，只有 <= now 才代表真正已经恢复。
    """
    ends_at = _parse_rfc3339(value)
    if ends_at is None:
        return False, None
    if ends_at <= now:
        return True, ends_at
    return False, None


def ingest_alert_webhook_alerts(alerts: list[dict[str, Any]]) -> dict[str, int]:
    """处理 Prometheus notifier 推送的 Alertmanager v2 格式告警数组。

    按 fingerprint 幂等 upsert：
    - endsAt 非零值 → 视为已恢复，直接采用 Prometheus 算出的恢复时间（比轮询精确）。
    - endsAt 为零值/缺失 → 仍在 firing：本地已有未 resolved 记录只刷新心跳，没有则新建。
    """
    now = timezone.now()
    created = 0
    resolved = 0
    heartbeats = 0
    notifications = 0

    for item in (alerts or []):
        labels = item.get('labels') if isinstance(item.get('labels'), dict) else {}
        annotations = item.get('annotations') if isinstance(item.get('annotations'), dict) else {}
        fingerprint = compute_alert_fingerprint(labels)
        is_resolved, ends_at = _resolve_ends_at(item.get('endsAt'), now)

        open_record = (
            AlertHistory.objects.filter(fingerprint=fingerprint, state=AlertHistory.State.FIRING)
            .order_by('-id')
            .first()
        )

        if is_resolved:
            if open_record is not None:
                open_record.state = AlertHistory.State.RESOLVED
                open_record.resolved_at = ends_at
                open_record.last_seen_at = now
                open_record.annotations = annotations
                open_record.save(update_fields=['state', 'resolved_at', 'last_seen_at', 'annotations', 'update_time'])
                resolved += 1
                notifications += int(_enqueue_notification(open_record, 'resolved'))
            # 本地没有对应 firing 记录（比如 backend 重启后错过了 firing 推送）：
            # 一条孤立的 resolved 消息没有历史意义，不建行。
            continue

        starts_at = _parse_rfc3339(item.get('startsAt')) or now
        if open_record is not None:
            open_record.last_seen_at = now
            open_record.labels = labels
            open_record.annotations = annotations
            open_record.save(update_fields=['last_seen_at', 'labels', 'annotations', 'update_time'])
            heartbeats += 1
            continue

        new_alert = AlertHistory.objects.create(
            fingerprint=fingerprint,
            alertname=str(labels.get('alertname') or ''),
            severity=str(labels.get('severity') or ''),
            instance=str(labels.get('instance') or ''),
            labels=labels,
            annotations=annotations,
            generator_url=str(item.get('generatorURL') or ''),
            state=AlertHistory.State.FIRING,
            started_at=starts_at,
            last_seen_at=now,
        )
        created += 1
        notifications += int(_enqueue_notification(new_alert, 'firing'))

    return {
        'created': created,
        'resolved': resolved,
        'heartbeats': heartbeats,
        'notifications': notifications,
    }


def reconcile_alert_history() -> int:
    """每日对账兜底：以 Prometheus 当前真实活跃告警为准，订正因推送丢失
    （backend/Prometheus 重启、网络抖动等）导致本地卡在 firing 的僵尸记录。

    只处理“本地 firing 但 Prometheus 已经没有”的记录，不影响已经通过 webhook
    正常恢复的历史行；恢复时间用对账发生的时间点，并标记 resolved_by_reconciliation=True，
    前端据此提示这是推测值而非精确恢复时间。
    """
    response = api_get('/api/v1/alerts')
    if not response.get('ok'):
        print(f"[RECONCILE] skip: fetch prometheus alerts failed: {response.get('error')}")
        return 0

    data = response.get('data') or {}
    raw_alerts = data.get('alerts') if isinstance(data, dict) else []
    active_fingerprints = set()
    for item in (raw_alerts or []):
        # 注意：/api/v1/alerts 返回的 state 是顶层字段，不是嵌套在 status 对象里
        # （之前的实现照搬了 Alertmanager v2 查询接口的结构，两者格式不一样）。
        state = str(item.get('state') or '').lower()
        if state != 'firing':
            continue
        labels = item.get('labels') if isinstance(item.get('labels'), dict) else {}
        active_fingerprints.add(compute_alert_fingerprint(labels))

    now = timezone.now()
    stale_qs = AlertHistory.objects.filter(state=AlertHistory.State.FIRING).exclude(fingerprint__in=active_fingerprints)
    stale_count = stale_qs.count()
    stale_qs.update(state=AlertHistory.State.RESOLVED, resolved_at=now, resolved_by_reconciliation=True, update_time=now)
    print(f'[RECONCILE] alert history reconciled: stale_resolved={stale_count}')
    return stale_count
