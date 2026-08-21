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


def get_prometheus_alert_rule_groups() -> dict[str, dict[str, Any]]:
    """Build rule-group indexes from Prometheus rules and their currently active alerts.

    Historical alerts no longer appear in a rule's active-alert list after recovery, so
    their alert name index is retained as the fallback association.
    """
    response = api_get('/api/v1/rules')
    if not response.get('ok'):
        return {'by_fingerprint': {}, 'by_alertname': {}}

    data = response.get('data') or {}
    raw_groups = data.get('groups') if isinstance(data, dict) else []
    by_fingerprint: dict[str, str] = {}
    by_alertname: dict[str, dict[str, Any]] = {}
    for raw_group in raw_groups or []:
        if not isinstance(raw_group, dict):
            continue
        group_name = str(raw_group.get('name') or '').strip()
        if not group_name:
            continue
        for raw_rule in raw_group.get('rules') or []:
            if not isinstance(raw_rule, dict):
                continue
            rule_name = str(raw_rule.get('name') or '').strip()
            if rule_name:
                by_alertname.setdefault(rule_name, {
                    'group_name': group_name,
                    'name': rule_name,
                    'query': raw_rule.get('query') or '',
                    'duration': raw_rule.get('duration'),
                    'labels': raw_rule.get('labels') if isinstance(raw_rule.get('labels'), dict) else {},
                    'annotations': raw_rule.get('annotations') if isinstance(raw_rule.get('annotations'), dict) else {},
                })
            for active_alert in raw_rule.get('alerts') or []:
                if not isinstance(active_alert, dict):
                    continue
                labels = active_alert.get('labels')
                if isinstance(labels, dict):
                    by_fingerprint[compute_alert_fingerprint(labels)] = group_name

    return {'by_fingerprint': by_fingerprint, 'by_alertname': by_alertname}


def resolve_alert_rule_group(
    labels: dict[str, Any] | None,
    alertname: str | None,
    rule_group_indexes: dict[str, dict[str, Any]],
) -> str:
    """Resolve a rule group using exact active-alert labels, then the rule name."""
    fingerprint = compute_alert_fingerprint(labels)
    by_fingerprint = rule_group_indexes.get('by_fingerprint') or {}
    by_alertname = rule_group_indexes.get('by_alertname') or {}
    exact_group = by_fingerprint.get(fingerprint)
    if exact_group:
        return str(exact_group)
    rule = by_alertname.get(str(alertname or '').strip()) or {}
    return str(rule.get('group_name') or '')


def resolve_alert_rule_details(
    labels: dict[str, Any] | None,
    alertname: str | None,
    rule_group_indexes: dict[str, dict[str, Any]],
) -> dict[str, Any]:
    """Resolve the Prometheus rule definition for an alert instance."""
    by_alertname = rule_group_indexes.get('by_alertname') or {}
    rule = by_alertname.get(str(alertname or '').strip())
    if not isinstance(rule, dict):
        return {}
    return dict(rule)


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
    rule_group_indexes = get_prometheus_alert_rule_groups()

    for item in (alerts or []):
        labels = item.get('labels') if isinstance(item.get('labels'), dict) else {}
        annotations = item.get('annotations') if isinstance(item.get('annotations'), dict) else {}
        fingerprint = compute_alert_fingerprint(labels)
        alertname = str(labels.get('alertname') or '')
        rule_group = resolve_alert_rule_group(labels, alertname, rule_group_indexes)
        rule_details = resolve_alert_rule_details(labels, alertname, rule_group_indexes)
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
                if not open_record.rule_group and rule_group:
                    open_record.rule_group = rule_group
                if not open_record.rule_snapshot and rule_details:
                    open_record.rule_snapshot = rule_details
                open_record.save(update_fields=[
                    'state', 'resolved_at', 'last_seen_at', 'annotations',
                    'rule_group', 'rule_snapshot', 'update_time',
                ])
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
            if not open_record.rule_group and rule_group:
                open_record.rule_group = rule_group
            if not open_record.rule_snapshot and rule_details:
                open_record.rule_snapshot = rule_details
            open_record.save(update_fields=[
                'last_seen_at', 'labels', 'annotations', 'rule_group', 'rule_snapshot', 'update_time',
            ])
            heartbeats += 1
            continue

        new_alert = AlertHistory.objects.create(
            fingerprint=fingerprint,
            alertname=alertname,
            rule_group=rule_group,
            rule_snapshot=rule_details,
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
    """每日对账告警状态，并清理已删除规则产生的未恢复记录。

    已恢复记录属于历史事实，不会被删除。对于本地仍为 firing 的记录：规则仍存在但
    活动告警消失时标记为对账恢复；规则本身已从 Prometheus 删除时直接删除该记录。
    """
    response = api_get('/api/v1/alerts')
    if not response.get('ok'):
        print(f"[RECONCILE] skip: fetch prometheus alerts failed: {response.get('error')}")
        return 0

    rules_response = api_get('/api/v1/rules')
    if not rules_response.get('ok'):
        print(f"[RECONCILE] skip rule deletion cleanup: fetch prometheus rules failed: {rules_response.get('error')}")
        current_rule_names = None
    else:
        rules_data = rules_response.get('data') or {}
        raw_groups = rules_data.get('groups') if isinstance(rules_data, dict) else []
        current_rule_names = {
            str(raw_rule.get('name') or '').strip()
            for raw_group in (raw_groups or [])
            if isinstance(raw_group, dict)
            for raw_rule in (raw_group.get('rules') or [])
            if isinstance(raw_rule, dict) and str(raw_rule.get('name') or '').strip()
        }

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
    deleted_count = 0
    if current_rule_names is not None:
        deleted_qs = stale_qs.exclude(alertname__in=current_rule_names)
        deleted_count = deleted_qs.count()
        deleted_qs.delete()
        stale_qs = stale_qs.filter(alertname__in=current_rule_names)

    resolved_count = stale_qs.count()
    stale_qs.update(state=AlertHistory.State.RESOLVED, resolved_at=now, resolved_by_reconciliation=True, update_time=now)
    print(
        '[RECONCILE] alert history reconciled: '
        f'stale_resolved={resolved_count}, deleted_rule_alerts={deleted_count}'
    )
    return resolved_count + deleted_count
