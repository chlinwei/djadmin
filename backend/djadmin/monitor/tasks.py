from datetime import timedelta

from django.db.models import Max
from django.utils import timezone

from sys_config.models import SysConfig

from .alert_history import reconcile_alert_history as _reconcile_alert_history
from .models import AlertHistory, MonitorTargetInstallHistory


def cleanup_monitor_install_histories():
    """清理过期监控安装历史，同时保证每个 target 至少保留最新一条记录。"""
    retention_cfg, _ = SysConfig.objects.get_or_create(
        key='sys.monitor.install_history.retention_days',
        defaults={
            'value': '180',
            'default_value': '180',
            'value_type': 'int',
            'name': '监控安装历史保留天数',
            'description': '监控安装/卸载历史记录保留天数，清理时每个纳管目标至少保留最新一条',
            'is_readonly': False,
        },
    )

    try:
        retention_days = max(1, int(str(retention_cfg.value).strip()))
    except (TypeError, ValueError):
        retention_days = 180

    cutoff = timezone.now() - timedelta(days=retention_days)

    # 每个 target 至少保留一条（最新一条）。
    latest_ids = list(
        MonitorTargetInstallHistory.objects
        .values('target_id')
        .annotate(latest_id=Max('id'))
        .values_list('latest_id', flat=True)
    )

    queryset = MonitorTargetInstallHistory.objects.filter(create_time__lt=cutoff)
    if latest_ids:
        queryset = queryset.exclude(id__in=latest_ids)

    deleted_rows = queryset.count()
    queryset.delete()
    print(
        '[CLEANUP] monitor install histories cleaned: '
        f'deleted={deleted_rows}, retention_days={retention_days}, keep_latest_per_target=true'
    )
    return deleted_rows


def reconcile_prometheus_alert_history():
    """每日对账兜底任务：见 alert_history.reconcile_alert_history 的详细说明。"""
    return _reconcile_alert_history()


def cleanup_alert_histories():
    """清理过期历史告警：只删已 resolved 且超过保留天数的行，仍在 firing 的记录永不清理。"""
    retention_cfg, _ = SysConfig.objects.get_or_create(
        key='sys.monitor.alert_history.retention_days',
        defaults={
            'value': '90',
            'default_value': '90',
            'value_type': 'int',
            'name': '历史告警保留天数',
            'description': '历史告警记录保留天数，只清理已恢复(resolved)的记录，仍在 firing 的记录不会被清理',
            'is_readonly': False,
        },
    )

    try:
        retention_days = max(1, int(str(retention_cfg.value).strip()))
    except (TypeError, ValueError):
        retention_days = 90

    cutoff = timezone.now() - timedelta(days=retention_days)
    queryset = AlertHistory.objects.filter(state=AlertHistory.State.RESOLVED, resolved_at__lt=cutoff)
    deleted_rows = queryset.count()
    queryset.delete()
    print(
        '[CLEANUP] alert histories cleaned: '
        f'deleted={deleted_rows}, retention_days={retention_days}'
    )
    return deleted_rows
