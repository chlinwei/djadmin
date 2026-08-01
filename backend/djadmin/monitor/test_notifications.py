from unittest.mock import patch

from celery.exceptions import Retry
from django.test import TestCase
from django.utils import timezone
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings

from assets.credential_crypto import decrypt_secret, encrypt_secret, is_encrypted_secret
from user.models import SysUser

from .alert_history import ingest_alert_webhook_alerts
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
            'recipient_emails': ['recipient@example.com'],
            'config': {
                'provider': 'gmail',
                'email': 'sender@gmail.com',
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


class AlertNotificationEventTest(TestCase):
    def _payload(self, ends_at='9999-12-31T23:59:59Z'):
        return [{
            'labels': {'alertname': 'HostDown', 'severity': 'critical', 'instance': '10.0.0.1'},
            'annotations': {'summary': 'Host is unavailable'},
            'startsAt': '2026-07-31T08:00:00Z',
            'endsAt': ends_at,
        }]

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
        self.user = SysUser.objects.create(username='alert-recipient', password='unused', status=1)
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
            recipient_emails=['recipient@example.com'],
        )
        route = AlertRoute.objects.create(
            name=f'Route {media.pk}',
            matchers=matchers or {},
            notify_on_firing=notify_on_firing,
            notify_on_resolved=notify_on_resolved,
        )
        route.media.add(media)
        return media

    @patch('monitor.tasks._send_email')
    def test_successful_delivery_marks_event_success(self, send_email):
        media = self._create_media()

        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.event.refresh_from_db()
        delivery = AlertNotificationDelivery.objects.get(event=self.event, media=media)
        self.assertEqual(result['status'], 'success')
        self.assertEqual(self.event.status, AlertNotificationEvent.Status.SUCCESS)
        self.assertEqual(delivery.status, 'success')
        self.assertEqual(delivery.attempt_count, 1)
        send_email.assert_called_once()

    def test_no_recipient_marks_event_failed_without_retry(self):
        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.event.refresh_from_db()
        self.assertEqual(result['status'], 'failed')
        self.assertEqual(self.event.status, AlertNotificationEvent.Status.FAILED)
        self.assertIn('告警路由', self.event.error_message)

    @patch('monitor.tasks._send_email')
    def test_non_matching_route_does_not_send(self, send_email):
        self._create_media(matchers={'severity': 'warning'})

        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.assertEqual(result['status'], 'failed')
        send_email.assert_not_called()

    @patch('monitor.tasks._send_email')
    def test_event_type_switch_excludes_firing(self, send_email):
        self._create_media(notify_on_firing=False, notify_on_resolved=True)

        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.assertEqual(result['status'], 'failed')
        send_email.assert_not_called()

    @patch('monitor.tasks._send_email', side_effect=OSError('SMTP unavailable'))
    def test_smtp_failure_records_delivery_and_retries(self, _send_email):
        media = self._create_media()

        with self.assertRaises(Retry):
            send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.event.refresh_from_db()
        delivery = AlertNotificationDelivery.objects.get(event=self.event, media=media)
        self.assertEqual(self.event.status, AlertNotificationEvent.Status.FAILED)
        self.assertEqual(delivery.status, 'failed')
        self.assertEqual(delivery.attempt_count, 1)
        self.assertIn('SMTP unavailable', delivery.error_message)

    @patch('monitor.tasks._send_email')
    def test_static_recipient_email_works_without_user_binding(self, send_email):
        media = AlertMedia.objects.create(
            name='Static Email',
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
            recipient_emails=['ops@example.com'],
        )
        route = AlertRoute.objects.create(
            name='Route Static',
            matchers={},
            notify_on_firing=True,
            notify_on_resolved=True,
        )
        route.media.add(media)

        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.event.refresh_from_db()
        delivery = AlertNotificationDelivery.objects.get(event=self.event, media=media, address='ops@example.com')
        self.assertEqual(result['status'], 'success')
        self.assertEqual(self.event.status, AlertNotificationEvent.Status.SUCCESS)
        self.assertEqual(delivery.status, 'success')
        self.assertIsNone(delivery.user)
        send_email.assert_called_once()

    @patch('monitor.tasks._send_email')
    def test_deduplicate_same_address_between_user_and_static_recipient(self, send_email):
        media = self._create_media()
        media.recipient_emails = ['recipient@example.com', 'recipient@example.com']
        media.save(update_fields=['recipient_emails', 'update_time'])

        result = send_alert_notification.run(self.event.pk)  # type: ignore[operator]

        self.assertEqual(result['status'], 'success')
        self.assertEqual(
            AlertNotificationDelivery.objects.filter(event=self.event, media=media, address='recipient@example.com').count(),
            1,
        )
        send_email.assert_called_once()