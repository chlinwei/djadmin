import io
import re
import zipfile
from django.test import TestCase
from django.utils import timezone
import importlib
from typing import Any, Callable, cast
from urllib.parse import unquote
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings
from django.apps import apps as django_apps
from datetime import timedelta

from .models import OperationAuditLog
from .models import LoginAuditLog
from assets.models import Host, WebSSHSessionLog
from menu.models import SysMenu, SysRoleMenu
from role.models import SysRole
from sys_config.models import SysConfig
from user.models import SysUser
from audit.tasks import cleanup_login_audit_logs, cleanup_operation_audit_logs

add_operation_audit_menu = importlib.import_module('menu.migrations.0014_add_operation_audit_menu').add_operation_audit_menu


def _get_token(user: SysUser) -> str:
    jwt_payload_handler = cast(Callable[[SysUser], Any], api_settings.JWT_PAYLOAD_HANDLER)
    jwt_encode_handler = cast(Callable[[Any], str], api_settings.JWT_ENCODE_HANDLER)
    payload = jwt_payload_handler(user)
    return jwt_encode_handler(payload)


class OperationAuditLogTest(TestCase):
    def setUp(self):
        self.client = APIClient()
        self.user = SysUser.objects.create(
            username='admin',
            password='admin123',
            status=1,
            email='admin@test.com',
            timezone='Asia/Shanghai',
        )
        self.admin_role = SysRole.objects.create(name='超级管理员', code='admin')
        add_operation_audit_menu(django_apps, None)
        token = _get_token(self.user)
        self.client.credentials(HTTP_AUTHORIZATION=token)

    def test_list_operation_logs_returns_paginated(self):
        OperationAuditLog.objects.create(
            username='admin',
            user_id=self.user.id,
            method='POST',
            path='/sys/usercenter/updateUserInfo/',
            route_name='usercenter-updateUserInfo',
            client_ip='127.0.0.1',
            user_agent='pytest',
            status_code=200,
            duration_ms=12,
            message='success',
        )

        res = self.client.get('/sys/audit/operation-logs/?page=1&page_size=10')
        body = res.json()
        self.assertEqual(body['code'], 200)
        self.assertIn('results', body['data'])
        self.assertIn('count', body['data'])
        self.assertGreaterEqual(body['data']['count'], 1)

    def test_operation_log_created_by_middleware(self):
        res = self.client.post('/sys/usercenter/updateUserInfo/', {
            'phonenumber': '13800138000',
            'email': 'center@test.com',
        }, format='json')
        self.assertEqual(res.json()['code'], 200)

        log = OperationAuditLog.objects.order_by('-id').first()
        self.assertIsNotNone(log)
        log = cast(OperationAuditLog, log)
        self.assertEqual(log.username, 'admin')
        self.assertEqual(log.path, '/sys/usercenter/updateUserInfo/')
        self.assertEqual(log.method, 'POST')
        self.assertIn('phonenumber', log.request_data)
        self.assertIn('center@test.com', log.request_data)
        self.assertIn('code', log.response_data)
        self.assertIn('200', log.response_data)

    def test_get_requests_are_not_logged(self):
        before_count = OperationAuditLog.objects.count()

        res = self.client.get('/sys/users/?page=1&page_size=10')
        self.assertEqual(res.json()['code'], 200)

        after_count = OperationAuditLog.objects.count()
        self.assertEqual(after_count, before_count)

    def test_operation_audit_menu_has_three_entries(self):
        audit_dir = SysMenu.objects.filter(path='/audit', menu_type='M').order_by('id').first()
        self.assertIsNotNone(audit_dir)
        audit_dir = cast(SysMenu, audit_dir)

        child_names = set(
            SysMenu.objects.filter(parent_id=audit_dir.id, menu_type='C').values_list('name', flat=True)
        )
        self.assertEqual(child_names, {'Web SSH会话日志', '登录日志', '操作日志'})

    def test_admin_role_has_operation_audit_menu_permissions(self):
        admin_role = cast(SysRole, self.admin_role)

        menu_ids = set(
            SysMenu.objects.filter(
                name__in={'Web SSH会话日志', '登录日志', '操作日志'},
                menu_type='C',
            ).values_list('id', flat=True)
        )
        self.assertEqual(len(menu_ids), 3)

        bound_menu_ids = set(
            SysRoleMenu.objects.filter(role_id=admin_role.id, menu_id__in=menu_ids).values_list('menu_id', flat=True)
        )
        self.assertEqual(bound_menu_ids, menu_ids)


class WebSSHAuditDownloadTest(TestCase):
    def setUp(self):
        self.client = APIClient()
        self.user = SysUser.objects.create(
            username='admin',
            password='admin123',
            status=1,
            email='admin@test.com',
            timezone='Asia/Shanghai',
        )
        add_operation_audit_menu(django_apps, None)
        token = _get_token(self.user)
        self.client.credentials(HTTP_AUTHORIZATION=token)

    def test_download_webssh_session_log(self):
        host = Host.objects.create(instance_name='ws_host', ip='192.168.1.10', port=22)
        session_log = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CLOSED,
            input_content='ls -la\n',
            output_content='total 0\n-rw-r--r-- test.txt\n',
            recorded_content_bytes=32,
            is_content_truncated=False,
        )

        res = self.client.get(f'/sys/audit/webssh-sessions/{session_log.id}/download/')  # type: ignore[attr-defined]

        self.assertEqual(res.status_code, 200)
        content_disposition = res.headers.get('Content-Disposition', '')
        self.assertIn('attachment', content_disposition)
        decoded_disposition = unquote(content_disposition)
        self.assertRegex(
            decoded_disposition,
            rf"webssh-{session_log.id}-ws_host\(192\.168\.1\.10\)-\d{{4}}-\d{{2}}-\d{{2}}-\d{{2}}-\d{{2}}-\d{{2}}\.log",  # type: ignore[attr-defined]
        )

        body = res.content.decode('utf-8')
        self.assertIn('Web SSH Session Log', body)
        self.assertIn('=== Output ===', body)
        self.assertIn('test.txt', body)
        self.assertNotIn('=== Input ===', body)
        self.assertNotIn('ls -la', body)

    def test_download_webssh_session_log_uses_sanitized_output(self):
        host = Host.objects.create(instance_name='ws_host_sanitized', ip='192.168.1.11', port=22)
        session_log = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CLOSED,
            output_content='line-a\r\n\x1b[31mline-b\x1b[0m\r\n',
            recorded_content_bytes=24,
            is_content_truncated=False,
        )

        content_res = self.client.get(f'/sys/audit/webssh-sessions/{session_log.id}/content/')  # type: ignore[attr-defined]
        self.assertEqual(content_res.status_code, 200)
        expected_output = content_res.json().get('data', {}).get('output_content', '')

        download_res = self.client.get(f'/sys/audit/webssh-sessions/{session_log.id}/download/')  # type: ignore[attr-defined]
        self.assertEqual(download_res.status_code, 200)
        body = download_res.content.decode('utf-8')

        self.assertIn(expected_output, body)
        self.assertNotIn('\x1b[31m', body)

    def test_download_filtered_webssh_session_logs(self):
        host = Host.objects.create(instance_name='ws_host2', ip='192.168.1.20', port=22)
        WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CLOSED,
            input_content='pwd\n',
            output_content='/home/admin\n',
            recorded_content_bytes=12,
            is_content_truncated=False,
        )
        WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.FAILED,
            input_content='whoami\n',
            output_content='error\n',
            recorded_content_bytes=10,
            is_content_truncated=False,
        )

        res = self.client.get('/sys/audit/webssh-sessions/download-all/?status=closed')

        self.assertEqual(res.status_code, 200)
        content_disposition = res.headers.get('Content-Disposition', '')
        self.assertIn('attachment', content_disposition)
        self.assertRegex(
            unquote(content_disposition),
            r"webssh-\d+-ws_host2\(192\.168\.1\.20\)-\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}\.log",
        )
        self.assertIn('.log', content_disposition)

        body = res.content.decode('utf-8')
        self.assertIn('Web SSH Session Log', body)
        self.assertIn('/home/admin', body)
        self.assertNotIn('error\n', body)
        self.assertIn('=== Output ===', body)
        self.assertNotIn('=== Input ===', body)

    def test_download_selected_webssh_session_logs_by_ids(self):
        host = Host.objects.create(instance_name='ws_host3', ip='192.168.1.30', port=22)
        selected_log = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CLOSED,
            input_content='echo selected\n',
            output_content='selected-output\n',
            recorded_content_bytes=16,
            is_content_truncated=False,
        )
        unselected_log = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CLOSED,
            input_content='echo unselected\n',
            output_content='unselected-output\n',
            recorded_content_bytes=18,
            is_content_truncated=False,
        )

        res = self.client.get(f'/sys/audit/webssh-sessions/download-all/?ids={selected_log.id}')  # type: ignore[attr-defined]

        self.assertEqual(res.status_code, 200)
        self.assertRegex(
            unquote(res.headers.get('Content-Disposition', '')),
            r"webssh-\d+-ws_host3\(192\.168\.1\.30\)-\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}\.log",
        )
        body = res.content.decode('utf-8')
        self.assertNotIn('Total: 1', body)
        self.assertIn('selected-output', body)
        self.assertNotIn('unselected-output', body)

    def test_download_webssh_session_logs_as_zip_when_more_than_two(self):
        host = Host.objects.create(instance_name='zip_host', ip='192.168.1.40', port=22)
        for index in range(3):
            WebSSHSessionLog.objects.create(
                host=host,
                user_id=self.user.id,
                username=self.user.username,
                status=WebSSHSessionLog.Status.CLOSED,
                input_content=f'echo {index}\n',
                output_content=f'output-{index}\n',
                recorded_content_bytes=10,
                is_content_truncated=False,
            )

        res = self.client.get('/sys/audit/webssh-sessions/download-all/?status=closed')

        self.assertEqual(res.status_code, 200)
        self.assertEqual(res.headers.get('Content-Type'), 'application/zip')
        content_disposition = res.headers.get('Content-Disposition', '')
        self.assertRegex(unquote(content_disposition), r"webssh-\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}\.zip")

        with zipfile.ZipFile(io.BytesIO(res.content), 'r') as zip_file:
            names = zip_file.namelist()
            self.assertEqual(len(names), 3)
            self.assertTrue(all(re.match(r'^webssh-\d+-.+\(.+\)-\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}\.log$', name) for name in names))
            first_content = zip_file.read(names[0]).decode('utf-8')
            self.assertIn('Web SSH Session Log', first_content)
            self.assertIn('=== Output ===', first_content)

    def test_list_webssh_sessions_filter_by_output_keyword(self):
        host = Host.objects.create(instance_name='filter_host', ip='192.168.1.50', port=22)
        matched = WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CLOSED,
            output_content='deploy succeeded with checksum ok\n',
            recorded_content_bytes=36,
            is_content_truncated=False,
        )
        WebSSHSessionLog.objects.create(
            host=host,
            user_id=self.user.id,
            username=self.user.username,
            status=WebSSHSessionLog.Status.CLOSED,
            output_content='connection timeout\n',
            recorded_content_bytes=19,
            is_content_truncated=False,
        )

        res = self.client.get('/sys/audit/webssh-sessions/?output_keyword=checksum&page=1&page_size=10')

        self.assertEqual(res.status_code, 200)
        body = res.json()
        self.assertEqual(body.get('code'), 200)
        results = body.get('data', {}).get('results', [])
        self.assertEqual(len(results), 1)
        self.assertEqual(results[0]['id'], matched.id)
        self.assertNotIn('session_id', results[0])


class AuditCleanupTaskTest(TestCase):
    def test_cleanup_login_audit_logs_respects_retention_window(self):
        SysConfig.objects.update_or_create(
            key='sys.audit.login_logs.retention_days',
            defaults={
                'value': '1',
                'default_value': '1',
                'value_type': 'int',
                'name': '登录日志保留天数',
                'description': 'test',
                'is_readonly': False,
            },
        )

        old_log = LoginAuditLog.objects.create(username='old-user')
        recent_log = LoginAuditLog.objects.create(username='recent-user')
        now = timezone.now()
        LoginAuditLog.objects.filter(id=old_log.id).update(login_time=now - timedelta(days=2))
        LoginAuditLog.objects.filter(id=recent_log.id).update(login_time=now - timedelta(hours=4))

        result = cleanup_login_audit_logs()

        self.assertEqual(result['login_retention_days'], 1)
        self.assertEqual(result['deleted_login'], 1)
        remaining_users = set(LoginAuditLog.objects.values_list('username', flat=True))
        self.assertIn('recent-user', remaining_users)
        self.assertNotIn('old-user', remaining_users)

    def test_cleanup_operation_audit_logs_respects_retention_window(self):
        SysConfig.objects.update_or_create(
            key='sys.audit.operation_logs.retention_days',
            defaults={
                'value': '1',
                'default_value': '1',
                'value_type': 'int',
                'name': '操作日志保留天数',
                'description': 'test',
                'is_readonly': False,
            },
        )

        old_log = OperationAuditLog.objects.create(username='old-op')
        recent_log = OperationAuditLog.objects.create(username='recent-op')
        now = timezone.now()
        OperationAuditLog.objects.filter(id=old_log.id).update(created_at=now - timedelta(days=2))
        OperationAuditLog.objects.filter(id=recent_log.id).update(created_at=now - timedelta(hours=4))

        result = cleanup_operation_audit_logs()

        self.assertEqual(result['operation_retention_days'], 1)
        self.assertEqual(result['deleted_operation'], 1)
        remaining_users = set(OperationAuditLog.objects.values_list('username', flat=True))
        self.assertIn('recent-op', remaining_users)
        self.assertNotIn('old-op', remaining_users)

    def test_cleanup_login_audit_logs_falls_back_to_default_on_invalid_config(self):
        SysConfig.objects.update_or_create(
            key='sys.audit.login_logs.retention_days',
            defaults={
                'value': 'abc',
                'default_value': '90',
                'value_type': 'int',
                'name': '登录日志保留天数',
                'description': 'test',
                'is_readonly': False,
            },
        )

        very_old = LoginAuditLog.objects.create(username='very-old-user')
        still_keep = LoginAuditLog.objects.create(username='still-keep-user')
        now = timezone.now()
        LoginAuditLog.objects.filter(id=very_old.id).update(login_time=now - timedelta(days=100))
        LoginAuditLog.objects.filter(id=still_keep.id).update(login_time=now - timedelta(days=30))

        result = cleanup_login_audit_logs()

        self.assertEqual(result['login_retention_days'], 90)
        self.assertEqual(result['deleted_login'], 1)
        remaining_users = set(LoginAuditLog.objects.values_list('username', flat=True))
        self.assertIn('still-keep-user', remaining_users)
        self.assertNotIn('very-old-user', remaining_users)