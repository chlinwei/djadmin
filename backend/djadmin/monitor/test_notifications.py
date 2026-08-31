from unittest.mock import patch

from celery.exceptions import Retry
from django.test import TestCase
from django.utils import timezone
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings

from assets.credential_crypto import decrypt_secret, encrypt_secret, is_encrypted_secret
from user.models import SysUser

from .alert_history import compute_alert_fingerprint, ingest_alert_webhook_alerts
from .models import AlertHistory, AlertMedia, AlertNotificationDelivery, AlertNotificationEvent, AlertRoute
from .tasks import send_alert_notification


def _get_token(user):
    payload = api_settings.JWT_PAYLOAD_HANDLER(user)  # type: ignore[operator]
    return api_settings.JWT_ENCODE_HANDLER(payload)  # type: ignore[operator]


class AlertMediaApiTest(TestCase):
    def setUp(self):
        self.client = APIClient()
        self.admin = SysUser.objects.create(username='admin', password='unused', status=1)
        self.client.credentials(HTTP_AUTHORIZATION=_get_token(self.admin))

    def test_gmail_crud_encrypts_and_preserves_password(self):
        create_response = self.client.post('/monitor/media/', {
            'name': 'Gmail Operations',
            'media_type': 'email',
            'enabled': True,
            'config': {
                'provider': 'gmail',
                'email': 'sender@gmail.com',
                'username': 'smtp-user@gmail.com',
                'password': 'app-password',
                'messageFormat': 'html',
            },
        }, format='json')

        create_body = create_response.json()
        self.assertEqual(create_response.status_code, 200)
        self.assertEqual(create_body['code'], 200)
        self.assertEqual(create_body['data']['config']['password'], '********')
        media = AlertMedia.objects.get(name='Gmail Operations')
        self.assertEqual(media.config['smtpServer'], 'smtp.gmail.com')
        self.assertEqual(media.config['smtpPort'], 587)
        self.assertEqual(media.config['authType'], 'password')
        self.assertNotIn('username', media.config)
        self.assertTrue(is_encrypted_secret(media.config['password']))
        self.assertEqual(decrypt_secret(media.config['password']), 'app-password')

        route_response = self.client.post('/monitor/alert-routes/', {
            'name': 'Critical alerts',
            'enabled': True,
            'matchers': {'severity': 'critical'},
            'notify_on_firing': True,
            'notify_on_resolved': True,
            'media': [media.pk],
        }, format='json')
        route_body = route_response.json()
        self.assertEqual(route_response.status_code, 200)
        self.assertEqual(route_body['code'], 200)
        self.assertEqual(route_body['data']['matchers'], {'severity': 'critical'})
        self.assertEqual(route_body['data']['media'], [media.pk])

        update_response = self.client.patch(f'/monitor/media/{media.pk}/', {
            'remark': 'Primary alert channel',
            'config': {
                **create_body['data']['config'],
                'password': '********',
            },
        }, format='json')
        self.assertEqual(update_response.status_code, 200)
        self.assertEqual(update_response.json()['code'], 200)
        media.refresh_from_db()
        self.assertEqual(decrypt_secret(media.config['password']), 'app-password')

        delete_response = self.client.delete(f'/monitor/media/{media.pk}/')
        self.assertEqual(delete_response.status_code, 200)
        self.assertEqual(delete_response.json(), {
            'code': 200,
            'data': {'deleted': True},
            'msg': 'success',
        })

    def test_custom_smtp_without_auth_does_not_require_username_or_password(self):
        response = self.client.post('/monitor/media/', {
            'name': 'Internal SMTP Relay',
            'media_type': 'email',
            'enabled': True,
            'config': {
                'provider': 'custom',
                'smtpServer': 'mail.example.com',
                'smtpPort': 25,
                'authType': 'none',
                'email': 'zabbix@example.com',
                'username': 'unused-user',
                'password': 'unused-password',
                'messageFormat': 'text',
            },
        }, format='json')

        self.assertEqual(response.status_code, 200)
        media = AlertMedia.objects.get(name='Internal SMTP Relay')
        self.assertEqual(media.config['authType'], 'none')
        self.assertNotIn('username', media.config)
        self.assertNotIn('password', media.config)

    @patch('monitor.tasks.EmailMultiAlternatives')
    @patch('monitor.tasks.get_connection')
    def test_email_media_test_sends_requested_message(self, get_connection, email_class):
        media = AlertMedia.objects.create(
            name='Test Email',
            media_type=AlertMedia.MediaType.EMAIL,
            config={
                'provider': 'custom',
                'smtpServer': 'smtp.example.com',
                'smtpPort': 25,
                'authType': 'none',
                'email': 'sender@example.com',
                'messageFormat': 'text',
            },
        )
        email_class.return_value.send.return_value = 1

        response = self.client.post(f'/monitor/media/{media.pk}/test/', {
            'recipients': ['one@example.com', 'two@example.com'],
            'subject': 'Test subject',
            'message': 'Test message',
        }, format='json')

        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json(), {
            'code': 200,
            'msg': 'success',
            'data': {'sent': True},
        })
        get_connection.assert_called_once()
        email_class.assert_called_once_with(
            subject='Test subject',
            body='Test message',
            from_email='sender@example.com',
            to=['one@example.com', 'two@example.com'],
            connection=get_connection.return_value,
        )


class AlertNotificationEventTest(TestCase):
    def setUp(self):
        self.media = AlertMedia.objects.create(
            name='ops-smtp', media_type=AlertMedia.MediaType.EMAIL, config={}, enabled=True,
        )
        route = AlertRoute.objects.create(name='all-alerts', enabled=True, matchers={})
        route.media.add(self.media)

    def _payload(self, ends_at='9999-12-31T23:59:59Z'):
        return [{
            'labels': {'alertname': 'HostDown', 'severity': 'critical', 'instance': '10.0.0.1'},
            'annotations': {'summary': 'Host is unavailable'},
            'startsAt': '2026-07-31T08:00:00Z',
            'endsAt': ends_at,
        }]

    @patch('monitor.tasks.send_alert_notification.delay')
    def test_no_enabled_media_produces_no_notification_event(self, delay):
        self.media.enabled = False
        self.media.save(update_fields=['enabled'])

        with self.captureOnCommitCallbacks(execute=True):  # type: ignore[attr-defined]
            result = ingest_alert_webhook_alerts(self._payload())

        # 没有可投递媒介时不该留下注定失败的通知记录，但告警本身仍要入库。
        self.assertEqual(result['notifications'], 0)
        self.assertEqual(AlertNotificationEvent.objects.count(), 0)
        self.assertEqual(AlertHistory.objects.count(), 1)
        delay.assert_not_called()

    @patch('monitor.tasks.send_alert_notification.delay')
    def test_firing_heartbeat_and_resolved_create_two_events(self, delay):
        with self.captureOnCommitCallbacks(execute=True):  # type: ignore[attr-defined]
            first = ingest_alert_webhook_alerts(self._payload())
        with self.captureOnCommitCallbacks(execute=True):  # type: ignore[attr-defined]
            heartbeat = ingest_alert_webhook_alerts(self._payload())
        with self.captureOnCommitCallbacks(execute=True):  # type: ignore[attr-defined]
            resolved = ingest_alert_webhook_alerts(self._payload('2026-07-31T08:10:00Z'))

        self.assertEqual(first['notifications'], 1)
        self.assertEqual(heartbeat['notifications'], 0)
        self.assertEqual(resolved['notifications'], 1)
        self.assertEqual(
            list(AlertNotificationEvent.objects.order_by('id').values_list('event_type', flat=True)),
            ['firing', 'resolved'],
        )
        self.assertEqual(delay.call_count, 2)


class AlertNotificationTaskTest(TestCase):
    def setUp(self):
        self.alert = AlertHistory.objects.create(
            fingerprint='notification-test',
            alertname='HostDown',
            severity='critical',
            instance='10.0.0.1',
            labels={'alertname': 'HostDown'},
            annotations={'summary': 'Host is unavailable'},
            state=AlertHistory.State.FIRING,
            started_at=timezone.now(),
            last_seen_at=timezone.now(),
        )
        self.event = AlertNotificationEvent.objects.create(
            alert=self.alert,
            event_type='firing',
            deduplication_key=f'{self.alert.pk}:firing',
        )

    def _create_media(self, matchers=None, notify_on_firing=True, notify_on_resolved=True):
        media = AlertMedia.objects.create(
            name='Operations Email',
            media_type=AlertMedia.MediaType.EMAIL,
            config={
                'provider': 'custom',
                'smtpServer': 'smtp.example.com',
                'smtpPort': 587,
                'email': 'sender@example.com',
                'password': encrypt_secret('app-password'),
                'messageFormat': 'text',
            },
            enabled=True,
        )
        route = AlertRoute.objects.create(
            name=f'Route {media.pk}',
            matchers=matchers or {},
            notify_on_firing=notify_on_firing,
            notify_on_resolved=notify_on_resolved,
        )
        route.media.add(media)
        
        # 为当前测试用户创建媒介绑定
        from user.models import SysUser
        user = SysUser.objects.first() or SysUser.objects.create(username='testuser', password='unused', status=1)
        from monitor.models import UserAlertMediaBinding
        UserAlertMediaBinding.objects.create(
            user=user,
            media=media,
            recipients=['ops@example.com'],
            enabled=True,
        )
        
        return media

    def test_no_recipient_marks_event_failed_without_retry(self):
        media = self._create_media()
        # 删除用户绑定以测试无收件人情况
        from monitor.models import UserAlertMediaBinding
        UserAlertMediaBinding.objects.filter(media=media).delete()

        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.event.refresh_from_db()
        self.assertEqual(result['status'], 'failed')
        self.assertEqual(self.event.status, AlertNotificationEvent.Status.FAILED)
        self.assertIn('没有任何用户绑定', self.event.error_message)

    def test_non_matching_route_does_not_send(self):
        self._create_media(matchers={'severity': 'warning'})

        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.assertEqual(result['status'], 'failed')

    def test_event_type_switch_excludes_firing(self):
        self._create_media(notify_on_firing=False, notify_on_resolved=True)

        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.assertEqual(result['status'], 'failed')

    @patch('monitor.tasks.EmailMultiAlternatives')
    @patch('monitor.tasks.get_connection')
    def test_send_alert_via_email_marks_success(self, mock_get_connection, mock_email):
        """验证成功发送电子邮件告警后，事件状态为 SUCCESS。"""
        self._create_media()

        # 配置模拟
        mock_email.return_value.send.return_value = 1

        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.event.refresh_from_db()
        self.assertEqual(result['status'], 'success')
        self.assertEqual(self.event.status, AlertNotificationEvent.Status.SUCCESS)
        mock_email.assert_called()
        self.assertEqual(mock_email.return_value.send.call_count, 1)
        delivery = AlertNotificationDelivery.objects.get(event=self.event)
        delivery_user = delivery.user
        self.assertIsNotNone(delivery_user)
        assert delivery_user is not None
        self.assertEqual(delivery_user.username, 'testuser')
        self.assertEqual(delivery.address, 'ops@example.com')
        self.assertEqual(delivery.status, AlertNotificationEvent.Status.SUCCESS)
        self.assertEqual(delivery.attempt_count, 1)
        self.assertIsNotNone(delivery.sent_at)

    @patch('monitor.tasks.EmailMultiAlternatives')
    @patch('monitor.tasks.get_connection')
    def test_retry_then_success_keeps_final_delivery_success(self, mock_get_connection, mock_email):
        self._create_media()
        mock_email.return_value.send.side_effect = [0, 1]

        with self.assertRaises(Retry):
            send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.event.refresh_from_db()
        self.assertEqual(self.event.status, AlertNotificationEvent.Status.PENDING)

        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.event.refresh_from_db()
        delivery = AlertNotificationDelivery.objects.get(event=self.event)
        self.assertEqual(result['status'], 'success')
        self.assertEqual(self.event.status, AlertNotificationEvent.Status.SUCCESS)
        self.assertEqual(delivery.status, AlertNotificationEvent.Status.SUCCESS)
        self.assertEqual(delivery.attempt_count, 2)
        self.assertEqual(delivery.error_message, '')


class AlertNotificationStatusApiTest(TestCase):
    def setUp(self):
        self.client = APIClient()
        self.admin = SysUser.objects.create(username='admin', password='unused', status=1)
        self.client.credentials(HTTP_AUTHORIZATION=_get_token(self.admin))
        self.alert = AlertHistory.objects.create(
            fingerprint='delivery-api-test',
            alertname='DiskFull',
            severity='critical',
            instance='10.0.0.2',
            labels={'alertname': 'DiskFull'},
            annotations={},
            state=AlertHistory.State.FIRING,
            started_at=timezone.now(),
            last_seen_at=timezone.now(),
        )
        self.event = AlertNotificationEvent.objects.create(
            alert=self.alert,
            event_type='firing',
            deduplication_key=f'{self.alert.pk}:firing',
            status=AlertNotificationEvent.Status.SUCCESS,
            attempt_count=1,
            sent_at=timezone.now(),
        )
        self.media = AlertMedia.objects.create(
            name='Delivery API Email',
            media_type=AlertMedia.MediaType.EMAIL,
            config={},
            enabled=True,
        )
        AlertNotificationDelivery.objects.create(
            event=self.event,
            media=self.media,
            user=self.admin,
            address='admin@example.com',
            status=AlertNotificationEvent.Status.SUCCESS,
            attempt_count=1,
            sent_at=timezone.now(),
        )

    def test_notification_status_returns_event_and_delivery_details(self):
        response = self.client.get(f'/monitor/alert-histories/{self.alert.pk}/notification-status/')

        self.assertEqual(response.status_code, 200)
        payload = response.json()
        self.assertEqual(payload['code'], 200)
        events = payload['data']['events']
        self.assertEqual(len(events), 1)
        self.assertEqual(events[0]['status'], AlertNotificationEvent.Status.SUCCESS)
        self.assertEqual(events[0]['deliveries'][0]['username'], self.admin.username)
        self.assertEqual(events[0]['deliveries'][0]['media_name'], self.media.name)
        self.assertEqual(events[0]['deliveries'][0]['address'], 'admin@example.com')

        list_response = self.client.get('/monitor/alert-histories/?page=1&page_size=10')
        list_row = list_response.json()['data']['results'][0]
        self.assertEqual(list_row['notification_status'], 'success')
        self.assertEqual(list_row['notification_count'], 1)
        self.assertEqual(list_row['notification_delivery_count'], 1)

    def test_history_list_filters_by_notification_summary_status(self):
        def create_alert(suffix):
            return AlertHistory.objects.create(
                fingerprint=f'delivery-filter-{suffix}',
                alertname=f'DeliveryFilter{suffix}',
                severity='warning',
                instance=f'10.0.1.{suffix}',
                labels={},
                annotations={},
                state=AlertHistory.State.FIRING,
                started_at=timezone.now(),
                last_seen_at=timezone.now(),
            )

        no_notification_alert = create_alert('1')
        legacy_alert = create_alert('2')
        AlertNotificationEvent.objects.create(
            alert=legacy_alert,
            event_type='firing',
            deduplication_key=f'{legacy_alert.pk}:firing',
            status=AlertNotificationEvent.Status.SUCCESS,
        )
        failed_alert = create_alert('3')
        AlertNotificationEvent.objects.create(
            alert=failed_alert,
            event_type='firing',
            deduplication_key=f'{failed_alert.pk}:firing',
            status=AlertNotificationEvent.Status.FAILED,
        )
        active_alert = create_alert('4')
        AlertNotificationEvent.objects.create(
            alert=active_alert,
            event_type='firing',
            deduplication_key=f'{active_alert.pk}:firing',
            status=AlertNotificationEvent.Status.PENDING,
        )

        expected_ids = {
            'none': {no_notification_alert.pk},
            'failed': {legacy_alert.pk, failed_alert.pk},
            'in_progress': {active_alert.pk},
            'success': {self.alert.pk},
        }
        for status, expected_status_ids in expected_ids.items():
            with self.subTest(status=status):
                response = self.client.get(
                    f'/monitor/alert-histories/?notification_status={status}&page=1&page_size=100'
                )
                result_ids = {row['id'] for row in response.json()['data']['results']}
                self.assertEqual(result_ids, expected_status_ids)


class PrometheusAlertNotificationMappingTest(TestCase):
    def setUp(self):
        self.client = APIClient()
        self.admin = SysUser.objects.create(username='admin', password='unused', status=1)
        self.client.credentials(HTTP_AUTHORIZATION=_get_token(self.admin))

    @patch('monitor.views.api_get')
    def test_pending_alert_does_not_reuse_previous_notification_history(self, api_get):
        labels = {'alertname': 'DiskFull', 'severity': 'critical', 'instance': '10.0.0.3'}
        old_alert = AlertHistory.objects.create(
            fingerprint=compute_alert_fingerprint(labels),
            alertname='DiskFull',
            severity='critical',
            instance='10.0.0.3',
            labels=labels,
            annotations={},
            state=AlertHistory.State.RESOLVED,
            started_at=timezone.now(),
            resolved_at=timezone.now(),
            last_seen_at=timezone.now(),
        )
        AlertNotificationEvent.objects.create(
            alert=old_alert,
            event_type='firing',
            deduplication_key=f'{old_alert.pk}:firing',
            status=AlertNotificationEvent.Status.SUCCESS,
        )
        api_get.return_value = {
            'ok': True,
            'data': {
                'alerts': [{
                    'state': 'pending',
                    'labels': labels,
                    'annotations': {'summary': 'Disk usage is high'},
                    'activeAt': timezone.now().isoformat(),
                    'value': '91',
                }],
            },
        }

        response = self.client.get('/monitor/targets/prometheus/alerts/')

        self.assertEqual(response.status_code, 200)
        row = response.json()['data']['results'][0]
        self.assertEqual(row['state'], 'pending')
        self.assertIsNone(row['history_id'])
        self.assertEqual(row['notification_count'], 0)
        self.assertEqual(row['notification_delivery_count'], 0)
        self.assertEqual(row['notification_status'], 'none')

