from datetime import timedelta
from typing import Any, Callable, cast
from unittest.mock import patch

from django.test import TestCase
from django.utils import timezone
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings

from menu.models import SysMenu
from scheduler.models import ScheduledTask, ScheduledTaskLog
from scheduler_manager import cleanup_task_logs, ensure_default_tasks, run_scheduled_task
from sys_config.models import SysConfig
from user.models import SysUser


class BaseSchedulerTestCase(TestCase):
    def setUp(self):
        self.client = APIClient()
        self.user = SysUser.objects.create(
            username='admin',
            password='admin123',
            status=1,
            email='admin@test.com',
            timezone='Asia/Shanghai',
        )
        token = self._get_token(self.user)
        self.client.credentials(HTTP_AUTHORIZATION=token)

    @staticmethod
    def _get_token(user: SysUser) -> str:
        jwt_payload_handler = cast(Callable[[SysUser], Any], api_settings.JWT_PAYLOAD_HANDLER)
        jwt_encode_handler = cast(Callable[[Any], str], api_settings.JWT_ENCODE_HANDLER)
        payload = jwt_payload_handler(user)
        return jwt_encode_handler(payload)

    @staticmethod
    def _create_task_menus():
        for index, path in enumerate([
            '/audit/webssh',
            '/sys/automation/logs',
            '/sys/automation/workflow',
            '/audit/login',
            '/audit/operation-log',
        ], start=1):
            SysMenu.objects.get_or_create(
                path=path,
                defaults={
                    'name': f'task-menu-{index}',
                    'menu_type': 'C',
                    'order_num': index,
                    'location': 1,
                    'parent_id': 0,
                },
            )


class SchedulerManagerTest(BaseSchedulerTestCase):
    def setUp(self):
        super().setUp()
        self._create_task_menus()

    def test_ensure_default_tasks_creates_split_tasks_and_removes_legacy(self):
        ScheduledTask.objects.create(name='旧审计日志清理', code='cleanup_audit_logs', enabled=True)

        ensure_default_tasks()

        expected_codes = {
            'cleanup_webssh_session_logs',
            'cleanup_ansible_execution_logs',
            'cleanup_login_audit_logs',
            'cleanup_operation_audit_logs',
        }
        actual_codes = set(ScheduledTask.objects.values_list('code', flat=True))
        self.assertTrue(expected_codes.issubset(actual_codes))
        self.assertFalse(ScheduledTask.objects.filter(code='cleanup_audit_logs').exists())

        login_task = ScheduledTask.objects.get(code='cleanup_login_audit_logs')
        operation_task = ScheduledTask.objects.get(code='cleanup_operation_audit_logs')
        self.assertEqual(login_task.menu.path, '/audit/login')
        self.assertEqual(operation_task.menu.path, '/audit/operation-log')

    def test_run_scheduled_task_writes_success_log(self):
        ensure_default_tasks()

        ok = run_scheduled_task('cleanup_login_audit_logs')

        self.assertTrue(ok)
        task = ScheduledTask.objects.get(code='cleanup_login_audit_logs')
        self.assertEqual(task.last_status, '成功')
        latest_log = ScheduledTaskLog.objects.filter(task=task).order_by('-id').first()
        self.assertIsNotNone(latest_log)
        self.assertEqual(latest_log.status, '成功')

    def test_cleanup_task_logs_applies_retention_and_max_rows(self):
        task = ScheduledTask.objects.create(name='日志清理测试任务', code='log-cleanup-test', enabled=True)

        SysConfig.objects.update_or_create(
            key='sys.scheduler.log_retention_days',
            defaults={
                'value': '1',
                'default_value': '1',
                'value_type': 'int',
                'name': '调度日志保留天数',
                'description': 'test',
                'is_readonly': False,
            },
        )
        SysConfig.objects.update_or_create(
            key='sys.scheduler.log_max_rows_per_task',
            defaults={
                'value': '2',
                'default_value': '2',
                'value_type': 'int',
                'name': '每任务最大日志条数',
                'description': 'test',
                'is_readonly': False,
            },
        )

        now = timezone.now()
        log_ids = []
        for idx in range(4):
            log = ScheduledTaskLog.objects.create(
                task=task,
                run_time=now - timedelta(hours=idx),
                status='成功',
                message=f'log-{idx}',
            )
            log_ids.append(log.id)

        old_log = ScheduledTaskLog.objects.create(
            task=task,
            run_time=now - timedelta(days=3),
            status='成功',
            message='too-old',
        )

        cleanup_task_logs(task)

        remaining = list(ScheduledTaskLog.objects.filter(task=task).order_by('-run_time').values_list('id', flat=True))
        self.assertEqual(len(remaining), 2)
        self.assertEqual(remaining, log_ids[:2])
        self.assertNotIn(old_log.id, remaining)


class ScheduledTaskApiTest(BaseSchedulerTestCase):
    def setUp(self):
        super().setUp()
        self._create_task_menus()
        ensure_default_tasks()

    def assert_api_ok(self, response):
        body = response.json()
        self.assertEqual(body.get('code'), 200, msg=body)
        self.assertIn('msg', body)
        self.assertIn('data', body)
        return body

    def test_list_supports_search(self):
        response = self.client.get('/sys/scheduler/tasks/?search=登录日志&page=1&page_size=10')

        body = self.assert_api_ok(response)
        rows = body['data']['results']
        self.assertGreaterEqual(len(rows), 1)
        self.assertTrue(any(row['code'] == 'cleanup_login_audit_logs' for row in rows))

    def test_update_ignores_menu_field(self):
        task = ScheduledTask.objects.get(code='cleanup_login_audit_logs')
        other_menu = SysMenu.objects.create(
            name='other-menu-for-update-test',
            path='/dummy/menu/for-test',
            menu_type='C',
            order_num=99,
            location=1,
            parent_id=0,
        )

        response = self.client.patch(
            f'/sys/scheduler/tasks/{task.id}/',
            {'menu': other_menu.id, 'enabled': False},
            format='json',
        )

        self.assertEqual(response.status_code, 200)
        body = response.json()
        task.refresh_from_db()
        self.assertFalse(task.enabled)
        self.assertEqual(task.menu.path, '/audit/login')
        self.assertEqual(body['menu_path'], '/audit/login')

    def test_update_accepts_description_field(self):
        task = ScheduledTask.objects.get(code='cleanup_login_audit_logs')

        response = self.client.patch(
            f'/sys/scheduler/tasks/{task.id}/',
            {'description': 'updated by test', 'cron_expression': '*/30 * * * *'},
            format='json',
        )

        self.assertEqual(response.status_code, 200)
        body = response.json()
        task.refresh_from_db()
        self.assertEqual(task.description, 'updated by test')
        self.assertEqual(task.cron_expression, '*/30 * * * *')
        self.assertEqual(body.get('description'), 'updated by test')

    def test_update_with_legacy_remark_field_does_not_change_description(self):
        task = ScheduledTask.objects.get(code='cleanup_login_audit_logs')
        task.description = 'origin description'
        task.save(update_fields=['description'])

        response = self.client.patch(
            f'/sys/scheduler/tasks/{task.id}/',
            {'remark': 'legacy field should fail'},
            format='json',
        )

        self.assertEqual(response.status_code, 200)
        task.refresh_from_db()
        self.assertEqual(task.description, 'origin description')

    @patch('scheduler.views.has_active_celery_worker', return_value=False)
    def test_run_now_returns_error_when_worker_offline(self, _mock_worker):
        task = ScheduledTask.objects.get(code='cleanup_login_audit_logs')

        response = self.client.post(f'/sys/scheduler/tasks/{task.id}/run_now/', {}, format='json')

        body = response.json()
        self.assertEqual(body.get('code'), 400)
        self.assertIn('Celery worker is not running', body.get('msg', ''))

    @patch('scheduler.views.execute_scheduled_task.delay')
    @patch('scheduler.views.has_active_celery_worker', return_value=True)
    def test_run_now_submits_task_when_worker_online(self, _mock_worker, mock_delay):
        task = ScheduledTask.objects.get(code='cleanup_login_audit_logs')

        response = self.client.post(f'/sys/scheduler/tasks/{task.id}/run_now/', {}, format='json')

        body = response.json()
        self.assertEqual(body.get('code'), 200)
        self.assertEqual(body.get('data', {}).get('status'), 'submitted')
        mock_delay.assert_called_once_with(task.code)


class WorkflowRunCleanupTaskTest(BaseSchedulerTestCase):
    """cleanup_workflow_run_logs 任务：函数行为 + run_now 接口"""

    def setUp(self):
        super().setUp()
        self._create_task_menus()
        ensure_default_tasks()

    def _make_workflow_run(self, status, days_ago):
        from automation.models import AutomationWorkflowRun, AutomationWorkflowTemplate
        workflow = AutomationWorkflowTemplate.objects.create(
            name=f'wf-{status}-{days_ago}d',
            enabled=True,
            nodes=[],
            edges=[],
        )
        run = AutomationWorkflowRun.objects.create(
            workflow=workflow,
            workflow_name_snapshot=workflow.name,
            status=status,
            node_results=[],
        )
        AutomationWorkflowRun.objects.filter(pk=run.pk).update(
            start_time=timezone.now() - timedelta(days=days_ago)
        )
        return run

    def test_cleanup_removes_finished_runs_older_than_retention(self):
        from automation.tasks import cleanup_workflow_run_logs

        SysConfig.objects.update_or_create(
            key='sys.workflow.logs.retention_days',
            defaults={
                'value': '7',
                'default_value': '7',
                'value_type': 'int',
                'name': 'Workflow 运行记录保留天数',
                'description': 'test',
                'is_readonly': False,
            },
        )

        # 应被删除：已完成且超过保留期
        old_success = self._make_workflow_run('success', days_ago=10)
        old_failed = self._make_workflow_run('failed', days_ago=8)
        # 应保留：已完成但未超期
        recent_success = self._make_workflow_run('success', days_ago=3)
        # 应保留：运行中，不管时间
        running = self._make_workflow_run('running', days_ago=10)

        deleted = cleanup_workflow_run_logs()

        self.assertEqual(deleted, 2)
        from automation.models import AutomationWorkflowRun
        remaining_ids = set(AutomationWorkflowRun.objects.values_list('id', flat=True))
        self.assertNotIn(old_success.id, remaining_ids)
        self.assertNotIn(old_failed.id, remaining_ids)
        self.assertIn(recent_success.id, remaining_ids)
        self.assertIn(running.id, remaining_ids)

    def test_run_scheduled_task_executes_workflow_cleanup(self):
        """run_scheduled_task 能正常调度 cleanup_workflow_run_logs 且写成功日志。"""
        ok = run_scheduled_task('cleanup_workflow_run_logs')

        self.assertTrue(ok)
        task = ScheduledTask.objects.get(code='cleanup_workflow_run_logs')
        self.assertEqual(task.last_status, '成功')
        log = ScheduledTaskLog.objects.filter(task=task).order_by('-id').first()
        self.assertIsNotNone(log)
        self.assertEqual(log.status, '成功')

    @patch('scheduler.views.execute_scheduled_task.delay')
    @patch('scheduler.views.has_active_celery_worker', return_value=True)
    def test_run_now_api_submits_workflow_cleanup_task(self, _mock_worker, mock_delay):
        """run_now 接口能正常提交 cleanup_workflow_run_logs 任务。"""
        task = ScheduledTask.objects.get(code='cleanup_workflow_run_logs')

        response = self.client.post(f'/sys/scheduler/tasks/{task.id}/run_now/', {}, format='json')

        body = response.json()
        self.assertEqual(body.get('code'), 200, msg=body)
        self.assertEqual(body.get('data', {}).get('status'), 'submitted')
        mock_delay.assert_called_once_with('cleanup_workflow_run_logs')
