"""巡检告警：把巡检结果接入既有告警通知链路（AlertHistory -> AlertRoute -> 媒介投递）。

按“任务 + 目标”维度维护一条告警的 firing/resolved 状态机，而不是每次执行都新建告警，
否则周期巡检会在告警历史里刷屏，且用户永远收不到“已恢复”的通知。
"""
from __future__ import annotations

from django.utils import timezone

from monitor.alert_history import compute_alert_fingerprint, enqueue_notification
from monitor.models import AlertHistory

from .models import InspectionExecution, InspectionResult, InspectionSeverity, InspectionTargetExecution

ALERT_NAME = 'InspectionFailed'


def _build_labels(execution):
    task_snapshot = execution.task_snapshot or {}
    service_snapshot = execution.service_snapshot or {}
    return {
        'alertname': ALERT_NAME,
        'inspection_task': str(task_snapshot.get('name') or ''),
        'inspection_task_id': str(task_snapshot.get('id') or ''),
        'inspection_target': str(service_snapshot.get('name') or ''),
        'inspection_target_type': str(service_snapshot.get('target_type') or ''),
    }


def _failed_targets(execution):
    return list(
        InspectionTargetExecution.objects
        .filter(execution=execution, status=InspectionTargetExecution.Status.FAILED)
        .values_list('target_name', 'error_message')
    )


def _warning_count(execution):
    return InspectionResult.objects.filter(
        target__execution=execution,
        severity=InspectionSeverity.WARNING,
    ).exclude(status__in=['pass', 'skipped']).count()


def sync_inspection_alert(execution):
    """执行收敛后同步告警状态；取消的执行不产生告警，返回本次是否新入队了通知。"""
    if execution.status == InspectionExecution.Status.CANCELED:
        return False

    labels = _build_labels(execution)
    fingerprint = compute_alert_fingerprint(labels)
    open_alert = (
        AlertHistory.objects
        .filter(fingerprint=fingerprint, state=AlertHistory.State.FIRING, source=AlertHistory.Source.INSPECTION)
        .order_by('-id')
        .first()
    )

    failed_targets = _failed_targets(execution)
    warning_count = _warning_count(execution)
    now = timezone.now()

    if not failed_targets and not warning_count:
        if open_alert is None:
            return False
        open_alert.state = AlertHistory.State.RESOLVED
        open_alert.resolved_at = now
        open_alert.last_seen_at = now
        open_alert.save(update_fields=['state', 'resolved_at', 'last_seen_at', 'update_time'])
        return enqueue_notification(open_alert, 'resolved')

    severity = InspectionSeverity.CRITICAL if failed_targets else InspectionSeverity.WARNING
    task_name = labels['inspection_task']
    failed_detail = '；'.join(
        f'{name}: {message}' if message else name
        for name, message in failed_targets[:10]
    )
    annotations = {
        'summary': f'巡检任务 {task_name} 发现异常',
        'description': (
            f'失败目标 {len(failed_targets)} 个，警告检查项 {warning_count} 个。'
            + (f' 失败明细：{failed_detail}' if failed_detail else '')
        ),
        'execution_id': str(execution.pk),
    }

    if open_alert is not None:
        # 同一异常持续存在时只刷新现场，不重复通知，避免周期巡检每轮发一封告警。
        open_alert.severity = severity
        open_alert.annotations = annotations
        open_alert.last_seen_at = now
        open_alert.save(update_fields=['severity', 'annotations', 'last_seen_at', 'update_time'])
        return False

    alert = AlertHistory.objects.create(
        source=AlertHistory.Source.INSPECTION,
        fingerprint=fingerprint,
        alertname=ALERT_NAME,
        rule_group='inspection',
        rule_snapshot={'task_id': labels['inspection_task_id'], 'task_name': task_name},
        severity=severity,
        instance=labels['inspection_target'],
        labels={**labels, 'severity': severity},
        annotations=annotations,
        state=AlertHistory.State.FIRING,
        started_at=now,
        last_seen_at=now,
    )
    return enqueue_notification(alert, 'firing')
