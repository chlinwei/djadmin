import smtplib
from email.message import EmailMessage

from datetime import timedelta

from django.db.models import Max
from django.utils import timezone
from celery import shared_task

from assets.credential_crypto import decrypt_secret
from sys_config.models import SysConfig

from .alert_history import reconcile_alert_history as _reconcile_alert_history
from .models import (
    AlertHistory,
    AlertMedia,
    AlertRoute,
    AlertNotificationEvent,
    AlertNotificationDelivery,
    MonitorTargetInstallHistory,
)


def _build_alert_email(event, alert):
    state_label = '恢复' if event.event_type == 'resolved' else '告警'
    subject = f'[{state_label}][{alert.severity or "unknown"}] {alert.alertname or "Prometheus Alert"}'
    lines = [
        f'告警名称：{alert.alertname or "-"}',
        f'状态：{event.event_type}',
        f'级别：{alert.severity or "-"}',
        f'实例：{alert.instance or "-"}',
        f'开始时间：{alert.started_at}',
        f'恢复时间：{alert.resolved_at or "-"}',
    ]
    if alert.annotations:
        lines.append('注释：')
        lines.extend(f'{key}：{value}' for key, value in alert.annotations.items())
    return subject, '\n'.join(lines)


def _send_email(media, recipient, subject, body):
    config = media.config if isinstance(media.config, dict) else {}
    smtp_server = str(config.get('smtpServer') or config.get('smtp_server') or '').strip()
    smtp_port = int(config.get('smtpPort') or config.get('smtp_port') or 0)
    username = str(config.get('email') or config.get('username') or '').strip()
    password = decrypt_secret(config.get('password')) or ''
    if not smtp_server or not smtp_port or not username or not password:
        raise ValueError(f'媒介 {media.name} 的 SMTP 配置不完整')

    message = EmailMessage()
    message['Subject'] = subject
    message['From'] = username
    message['To'] = recipient
    message.set_content(body)
    if config.get('messageFormat') == 'html':
        html_body = '<br>'.join(body.replace('&', '&amp;').replace('<', '&lt;').replace('>', '&gt;').splitlines())
        message.add_alternative(html_body, subtype='html')

    if smtp_port == 465:
        with smtplib.SMTP_SSL(smtp_server, smtp_port, timeout=15) as client:
            client.login(username, password)
            client.send_message(message)
        return

    with smtplib.SMTP(smtp_server, smtp_port, timeout=15) as client:
        client.ehlo()
        client.starttls()
        client.ehlo()
        client.login(username, password)
        client.send_message(message)


def _resolve_event_media(event):
    alert = event.alert
    labels = {str(key): str(value) for key, value in (alert.labels or {}).items()}
    labels.update({
        'alertname': str(alert.alertname or ''),
        'severity': str(alert.severity or ''),
        'instance': str(alert.instance or ''),
    })
    media_ids = set()
    routes = AlertRoute.objects.filter(enabled=True).prefetch_related('media')
    for route in routes:
        if event.event_type == 'firing' and not route.notify_on_firing:
            continue
        if event.event_type == 'resolved' and not route.notify_on_resolved:
            continue
        matchers = route.matchers if isinstance(route.matchers, dict) else {}
        if all(labels.get(str(key)) == str(value) for key, value in matchers.items()):
            media_ids.update(route.media.filter(enabled=True).values_list('id', flat=True))
    return AlertMedia.objects.filter(id__in=media_ids, enabled=True).prefetch_related('users')


@shared_task(bind=True, name='monitor.send_alert_notification', max_retries=5)
def send_alert_notification(self, event_id):
    event = AlertNotificationEvent.objects.select_related('alert').get(id=event_id)
    if event.status == AlertNotificationEvent.Status.SUCCESS:
        return {'status': 'success', 'event_id': event.pk}

    event.status = AlertNotificationEvent.Status.SENDING
    event.attempt_count += 1
    event.save(update_fields=['status', 'attempt_count', 'update_time'])

    subject, body = _build_alert_email(event, event.alert)
    deliveries = []
    for media in _resolve_event_media(event):
        if media.media_type != AlertMedia.MediaType.EMAIL:
            continue
        for user in media.users.filter(status=1):
            address = str(user.email or '').strip()
            if not address:
                continue
            delivery, _ = AlertNotificationDelivery.objects.get_or_create(
                event=event,
                media=media,
                user=user,
                address=address,
            )
            if delivery.status == 'success':
                deliveries.append(delivery)
                continue
            delivery.attempt_count += 1
            try:
                _send_email(media, address, subject, body)
                delivery.status = 'success'
                delivery.sent_at = timezone.now()
                delivery.error_message = ''
            except Exception as exc:
                delivery.status = 'failed'
                delivery.error_message = str(exc)
            delivery.save(update_fields=['status', 'sent_at', 'error_message', 'attempt_count', 'update_time'])
            deliveries.append(delivery)

    if not deliveries:
        event.status = AlertNotificationEvent.Status.FAILED
        event.error_message = '没有匹配到包含有效 Email 收件人的启用告警路由'
        event.save(update_fields=['status', 'error_message', 'update_time'])
        return {'status': 'failed', 'event_id': event.pk}

    failed = [item for item in deliveries if item.status != 'success']
    if failed:
        event.status = AlertNotificationEvent.Status.FAILED
        event.error_message = '; '.join(item.error_message for item in failed if item.error_message)
        event.save(update_fields=['status', 'error_message', 'update_time'])
        if self.request.retries < self.max_retries:
            raise self.retry(countdown=min(300, 2 ** self.request.retries * 10))
        return {'status': 'failed', 'event_id': event.pk}

    event.status = AlertNotificationEvent.Status.SUCCESS
    event.sent_at = timezone.now()
    event.error_message = ''
    event.save(update_fields=['status', 'sent_at', 'error_message', 'update_time'])
    return {'status': 'success', 'event_id': event.pk}


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
