from datetime import timedelta

from django.core.mail import EmailMultiAlternatives, get_connection
from django.db.models import Max, Prefetch
from django.utils import timezone
from celery import shared_task

from assets.credential_crypto import decrypt_secret
from sys_config.models import SysConfig

from .alert_history import reconcile_alert_history as _reconcile_alert_history
from .models import (
    AlertHistory,
    AlertMedia,
    AlertRoute,
	AlertNotificationDelivery,
    AlertNotificationEvent,
    UserAlertMediaBinding,
    MonitorTargetInstallHistory,
)


def resolve_alert_media(alert, event_type):
    """返回这条告警在此事件类型下实际可投递的媒介；入队前与发送时共用同一套匹配规则。"""
    labels = {str(key): str(value) for key, value in (alert.labels or {}).items()}
    labels.update({
        'alertname': str(alert.alertname or ''),
        'severity': str(alert.severity or ''),
        'instance': str(alert.instance or ''),
    })
    media_ids = set()
    # 在 Prefetch 里就筛掉停用媒介；若改用 route.media.filter() 会绕过预取缓存，每条命中路由多一次查询。
    routes = AlertRoute.objects.filter(enabled=True).prefetch_related(
        Prefetch('media', queryset=AlertMedia.objects.filter(enabled=True)),
    )
    for route in routes:
        if event_type == 'firing' and not route.notify_on_firing:
            continue
        if event_type == 'resolved' and not route.notify_on_resolved:
            continue
        matchers = route.matchers if isinstance(route.matchers, dict) else {}
        if all(labels.get(str(key)) == str(value) for key, value in matchers.items()):
            media_ids.update(media.id for media in route.media.all())
    return AlertMedia.objects.filter(id__in=media_ids)


def _send_email_alert(media, alert, recipients):
	"""发送告警邮件。返回 (success: bool, error_message: str)。"""
	if not recipients:
		return False, '未配置收件人邮箱'

	config = media.config or {}
	try:
		password = decrypt_secret(config.get('password', '')) if config.get('password') else ''
		connection = get_connection(
			backend='django.core.mail.backends.smtp.EmailBackend',
			fail_silently=False,
			host=config.get('smtpServer'),
			port=int(config.get('smtpPort', 587)),
			username=config.get('username'),
			password=password,
			use_tls=bool(config.get('useTLS', False)),
			use_ssl=bool(config.get('useSSL', False)),
		)
		
		# 构建邮件主题和内容
		alert_name = alert.alertname or 'Unknown Alert'
		severity = alert.severity or 'unknown'
		state_label = 'Firing' if alert.state == AlertHistory.State.FIRING else 'Resolved'
		subject = f'[{state_label}] {alert_name} - {severity}'
		
		message_body = f"""
Alert: {alert_name}
Severity: {severity}
State: {state_label}
Instance: {alert.instance or 'N/A'}

Started: {alert.started_at}
Last Seen: {alert.last_seen_at}
{f'Resolved: {alert.resolved_at}' if alert.resolved_at else ''}

Labels: {alert.labels}
Annotations: {alert.annotations}
Generator URL: {alert.generator_url}
"""
		
		email = EmailMultiAlternatives(
			subject=subject,
			body=message_body,
			from_email=config.get('email'),
			to=recipients,
			connection=connection,
		)
		if config.get('messageFormat', 'text') == 'html':
			email.attach_alternative(message_body.replace('\n', '<br>'), 'text/html')
		
		sent_count = email.send()
		if sent_count == 1:
			return True, ''
		else:
			return False, f'邮件发送返回异常计数: {sent_count}'
	except Exception as exc:
		return False, f'邮件发送失败: {str(exc)}'


@shared_task(bind=True, name='monitor.send_alert_notification', max_retries=5)
def send_alert_notification(self, event_id):
	"""按用户、媒介和地址发送告警通知，并持久化每次实际投递结果。"""
	event = AlertNotificationEvent.objects.select_related('alert').get(id=event_id)
	if event.status == AlertNotificationEvent.Status.SUCCESS:
		return {'status': 'success', 'event_id': event.pk}

	event.status = AlertNotificationEvent.Status.SENDING
	event.attempt_count += 1
	event.save(update_fields=['status', 'attempt_count', 'update_time'])

	alert = event.alert
	medias = resolve_alert_media(alert, event.event_type)
	
	if not medias.exists():
		event.status = AlertNotificationEvent.Status.FAILED
		# 入队时已筛过一次，走到这里说明媒介在入队后被停用或路由被改。
		event.error_message = '媒介在入队后被停用或路由已变更'
		event.save(update_fields=['status', 'error_message', 'update_time'])
		return {'status': 'failed', 'event_id': event.pk}

	errors = []
	retryable_errors = []
	delivery_count = 0
	for media in medias:
		if media.media_type != AlertMedia.MediaType.EMAIL:
			# 当前仅实现 Email，webhook 后续补充
			continue
		
		# 获取所有对该媒介有有效绑定的用户及其收件人
		bindings = UserAlertMediaBinding.objects.filter(
			media=media, enabled=True
		).select_related('user')
		
		if not bindings.exists():
			errors.append(f'媒介 "{media.name}" 没有任何用户绑定')
			continue
		
		# 每个用户地址单独投递，才能准确记录“谁、通过什么媒介、是否发送成功”。
		for binding in bindings:
			recipients = binding.recipients if isinstance(binding.recipients, list) else []
			for recipient in dict.fromkeys(str(item).strip() for item in recipients if str(item).strip()):
				delivery, _ = AlertNotificationDelivery.objects.get_or_create(
					event=event,
					media=media,
					user=binding.user,
					address=recipient,
				)
				delivery_count += 1
				if delivery.status == AlertNotificationEvent.Status.SUCCESS:
					continue

				delivery.status = AlertNotificationEvent.Status.SENDING
				delivery.attempt_count += 1
				delivery.error_message = ''
				delivery.save(update_fields=['status', 'attempt_count', 'error_message', 'update_time'])

				success, error_msg = _send_email_alert(media, alert, [recipient])
				if success:
					delivery.status = AlertNotificationEvent.Status.SUCCESS
					delivery.sent_at = timezone.now()
					delivery.error_message = ''
				else:
					delivery.status = AlertNotificationEvent.Status.FAILED
					delivery.error_message = error_msg
					delivery_error = f'{binding.user.username} / {media.name} / {recipient}: {error_msg}'
					errors.append(delivery_error)
					retryable_errors.append(delivery_error)
				delivery.save(update_fields=['status', 'sent_at', 'error_message', 'update_time'])

	if delivery_count == 0 and not errors:
		errors.append('匹配的告警媒介没有可投递的用户地址')

	if errors:
		event.error_message = '; '.join(errors)
		# 重试机制：指数退避
		if retryable_errors and self.request.retries < self.max_retries:
			event.status = AlertNotificationEvent.Status.PENDING
			event.save(update_fields=['status', 'error_message', 'update_time'])
			raise self.retry(countdown=min(300, 2 ** self.request.retries * 10))
		event.status = AlertNotificationEvent.Status.FAILED
		event.save(update_fields=['status', 'error_message', 'update_time'])
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
	"""每 5 分钟对账：规则仍存在的失联告警标记恢复，已删除规则的未恢复记录直接删除。"""
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
