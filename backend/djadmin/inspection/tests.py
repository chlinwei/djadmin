import json
from unittest.mock import MagicMock, patch

from django.test import TransactionTestCase
from django.utils import timezone
from rest_framework.test import APIRequestFactory, force_authenticate
from rest_framework_jwt.settings import api_settings

from assets.models import (
    Application,
    ApplicationDeployment,
    ApplicationDeploymentTemplate,
    ApplicationService,
    ApplicationServiceDeployment,
    ApplicationVersion,
    BusinessEnvironment,
    BusinessSystem,
    ClusterProfile,
    Host,
    HostGroup,
)
from user.models import SysUser

from .alerting import sync_inspection_alert
from .executor import _agent_params, _select_service_agent, run_inspection_execution
from .models import (
    InspectionExecution,
    InspectionGroup,
    InspectionResult,
    InspectionSeverity,
    InspectionTargetExecution,
    InspectionTask,
)
from .serializers import InspectionGroupSerializer, InspectionTaskSerializer
from .scheduling import calculate_next_run_time, dispatch_scheduled_inspections, fail_stale_inspection_executions
from .views import InspectionTaskViewSet


def _get_token(user):
    jwt_payload_handler = api_settings.JWT_PAYLOAD_HANDLER  # type: ignore[operator]
    jwt_encode_handler = api_settings.JWT_ENCODE_HANDLER  # type: ignore[operator]
    return jwt_encode_handler(jwt_payload_handler(user))  # type: ignore[operator]


def _run_request(task_pk, user):
    """带 JWT 的 run 请求：巡检接口已启用菜单权限，无 token 会直接被拒。"""
    request = APIRequestFactory().post(
        f'/sys/inspection/tasks/{task_pk}/run/', {}, format='json',
        HTTP_AUTHORIZATION=_get_token(user),
    )
    force_authenticate(request, user=user)
    return request


class InspectionTestCase(TransactionTestCase):
    def setUp(self):
        # 告警入队会在事务提交后投递 Celery 通知，测试环境没有 broker，必须挡在这里。
        notification_patcher = patch('monitor.tasks.send_alert_notification')
        self.send_alert_notification = notification_patcher.start()
        self.addCleanup(notification_patcher.stop)
        self.business_system = BusinessSystem.objects.create(name='巡检系统', code='inspection-system')
        self.environment = BusinessEnvironment.objects.create(name='测试环境', code='testing')
        self.application = Application.objects.create(name='Demo App', code='demo-app')
        self.version = ApplicationVersion.objects.create(application=self.application, version='1.2.3')
        self.template = ApplicationDeploymentTemplate.objects.create(
            application=self.application, name='default', control_type='command', run_user='demo',
            app_home='/opt/demo', service_name='demo.service',
        )
        self.service = ApplicationService.objects.create(
            business_system=self.business_system,
            environment=self.environment,
            application=self.application,
            application_version=self.version,
            deployment_template=self.template,
            name='Demo Service',
            code='demo-service',
        )

    def create_group(self, scope=InspectionGroup.Scope.SERVICE_ONCE):
        executor = 'http' if scope == InspectionGroup.Scope.SERVICE_ONCE else 'shell'
        config = {'url': 'http://service.test/health', 'expected_status': 200} if executor == 'http' else {
            'command': 'echo ok', 'work_directory': '${APP_HOME}', 'expected_output': 'ok',
        }
        serializer = InspectionGroupSerializer(data={
            'name': f'group-{scope}', 'scope': scope,
            'checks': [{'name': 'health', 'executor': executor, 'config': config, 'enabled': True, 'order': 0}],
        })
        self.assertTrue(serializer.is_valid(), serializer.errors)
        return serializer.save()

    def create_host_group_check(self):
        serializer = InspectionGroupSerializer(data={
            'name': 'host-check',
            'scope': InspectionGroup.Scope.PER_DEPLOYMENT,
            'checks': [{
                'name': 'hostname',
                'executor': 'shell',
                'config': {'command': 'echo ${HOST_NAME}', 'work_directory': '/', 'expected_output': '${HOST_NAME}'},
                'enabled': True,
                'order': 0,
            }],
        })
        self.assertTrue(serializer.is_valid(), serializer.errors)
        return serializer.save()

    def test_group_allows_http_for_each_deployment(self):
        serializer = InspectionGroupSerializer(data={
            'name': 'deployment-http', 'scope': InspectionGroup.Scope.PER_DEPLOYMENT,
            'checks': [{'name': 'http', 'executor': 'http', 'config': {'url': 'http://localhost'}}],
        })
        self.assertTrue(serializer.is_valid(), serializer.errors)

    def test_group_allows_tcp_without_host(self):
        serializer = InspectionGroupSerializer(data={
            'name': 'deployment-local-tcp', 'scope': InspectionGroup.Scope.PER_DEPLOYMENT,
            'checks': [{'name': 'local-port', 'executor': 'tcp', 'config': {'host': '', 'port': 8080}}],
        })
        self.assertTrue(serializer.is_valid(), serializer.errors)

    def test_group_validates_schema_configuration(self):
        serializer = InspectionGroupSerializer(data={
            'name': 'deployment-schema', 'scope': InspectionGroup.Scope.PER_DEPLOYMENT,
            'checks': [{
                'name': 'server.xml',
                'executor': 'schema_validate',
                'config': {
                    'path': '${APP_HOME}/conf/server.xml',
                    'document_type': 'xml',
                    'schema_type': 'schematron',
                    'schema_content': '<schema />',
                },
            }],
        })
        self.assertTrue(serializer.is_valid(), serializer.errors)

    @patch('inspection.executor.AgentChannelClient')
    def test_service_execution_runs_http_check_on_agent(self, client_class):
        group = self.create_group()
        task = InspectionTask.objects.create(name='controller-task', group=group, logical_service=self.service)
        host = Host.objects.create(instance_name='node-1', ip='10.0.0.8', agent_id='agent-1', agent_online=True)
        deployment = ApplicationDeployment.objects.create(host=host, instance_name='demo-1')
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=deployment)
        execution = InspectionExecution.objects.create(
            task=task,
            task_snapshot={'timeout_seconds': 10, 'concurrency': 1},
            group_snapshot={'scope': group.scope, 'checks': list(group.checks.values('name', 'executor', 'config'))},
            service_snapshot={'name': self.service.name, 'cluster_type': '', 'access_address': ''},
            target_snapshot=[{'deployment_id': deployment.pk}],
        )
        target = InspectionTargetExecution.objects.create(execution=execution, target_name=self.service.name)
        client_class.return_value.execute_automation.return_value = {
            'result_data': {
                'passed': True,
                'checks': [{
                    'key': f'inspection:{execution.pk}:0', 'type': 'http', 'name': 'health',
                    'status': 'pass', 'expected': 200, 'actual': 200,
                }],
            },
            'error_message': '',
        }

        run_inspection_execution(execution.pk)

        execution.refresh_from_db()
        target.refresh_from_db()
        self.assertEqual(execution.status, InspectionExecution.Status.SUCCESS)
        self.assertTrue(target.passed)
        self.assertEqual(InspectionResult.objects.get(target=target).status, 'pass')
        self.assertEqual(target.agent_id_snapshot, 'agent-1')
        params = client_class.return_value.execute_automation.call_args.kwargs['params']
        self.assertEqual(params['check_plan']['required_capabilities'], ['http:v1'])
        self.assertEqual(params['check_plan']['checks'][0]['url'], 'http://service.test/health')

    @patch('inspection.executor.AgentChannelClient')
    def test_service_execution_selects_agent_that_owns_ha_vip(self, client_class):
        profile = ClusterProfile.objects.create(
            name='ha-profile', profile_type=ClusterProfile.ProfileType.BUILTIN,
            cluster_type=ClusterProfile.ClusterType.HA,
        )
        self.service.cluster_profile = profile
        self.service.access_address = '10.0.0.100'
        self.service.save(update_fields=['cluster_profile', 'access_address', 'update_time'])
        host_a = Host.objects.create(instance_name='node-a', ip='10.0.0.11', agent_id='agent-a', agent_online=True)
        host_b = Host.objects.create(instance_name='node-b', ip='10.0.0.12', agent_id='agent-b', agent_online=True)
        deployment_a = ApplicationDeployment.objects.create(host=host_a, instance_name='demo-a')
        deployment_b = ApplicationDeployment.objects.create(host=host_b, instance_name='demo-b')
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=deployment_a)
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=deployment_b)
        execution = InspectionExecution.objects.create(
            task_snapshot={'timeout_seconds': 10},
            service_snapshot={'name': self.service.name, 'cluster_type': 'ha', 'access_address': '10.0.0.100'},
            target_snapshot=[{'deployment_id': deployment_a.pk}, {'deployment_id': deployment_b.pk}],
        )
        target = InspectionTargetExecution.objects.create(execution=execution, target_name=self.service.name)

        def agent_client(agent_id, timeout=None):
            client = MagicMock()
            addresses = ['10.0.0.100', '10.0.0.12'] if agent_id == 'agent-b' else ['10.0.0.11']
            client.execute_automation.return_value = {'result_data': {'local_ipv4': addresses}}
            return client

        client_class.side_effect = agent_client

        _select_service_agent(execution, target)

        target.refresh_from_db()
        self.assertEqual(target.deployment_id, deployment_b.pk)
        self.assertEqual(target.agent_id_snapshot, 'agent-b')

    def test_agent_plan_resolves_deployment_variables(self):
        group = self.create_group(InspectionGroup.Scope.PER_DEPLOYMENT)
        host = Host.objects.create(instance_name='node-1', ip='10.0.0.8', agent_id='agent-1', agent_online=True)
        deployment = ApplicationDeployment.objects.create(host=host, instance_name='demo-1')
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=deployment)
        execution = InspectionExecution.objects.create(
            task_snapshot={'timeout_seconds': 10},
            group_snapshot={'scope': group.scope, 'checks': list(group.checks.values('name', 'executor', 'config'))},
        )

        params = _agent_params(execution, deployment)

        self.assertEqual(params['control_type'], 'command')
        check = params['check_plan']['checks'][0]
        self.assertEqual(check['work_directory'], '/opt/demo')
        self.assertEqual(check['environment']['INSTANCE_NAME'], 'demo-1')

        execution.group_snapshot['checks'][0]['config']['expected_output'] = 'Apache Tomcat/${APPLICATION_VERSION}'
        execution.save(update_fields=['group_snapshot'])
        resolved_check = _agent_params(execution, deployment)['check_plan']['checks'][0]
        self.assertEqual(resolved_check['expected'], 'Apache Tomcat/1.2.3')

    def test_agent_plan_compiles_schema_without_expanding_rule_content(self):
        host = Host.objects.create(instance_name='node-schema', ip='10.0.0.18', agent_id='agent-schema', agent_online=True)
        deployment = ApplicationDeployment.objects.create(host=host, instance_name='demo-schema')
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=deployment)
        execution = InspectionExecution.objects.create(
            task_snapshot={'timeout_seconds': 10},
            group_snapshot={'scope': InspectionGroup.Scope.PER_DEPLOYMENT, 'checks': [{
                'name': 'server.xml',
                'executor': 'schema_validate',
                'config': {
                    'path': '${APP_HOME}/conf/server.xml',
                    'document_type': 'xml',
                    'schema_type': 'schematron',
                    'schema_content': '<assert test="${APPLICATION_VERSION}">version</assert>',
                },
            }]},
        )

        params = _agent_params(execution, deployment)
        check = params['check_plan']['checks'][0]

        self.assertEqual(params['check_plan']['required_capabilities'], ['schema_validate:v1'])
        self.assertEqual(check['path'], '/opt/demo/conf/server.xml')
        self.assertEqual(check['schema']['version'], 'iso')
        self.assertIn('${APPLICATION_VERSION}', check['schema']['content'])

    def test_agent_plan_defaults_empty_tcp_host_to_agent_localhost(self):
        host = Host.objects.create(instance_name='node-tcp', ip='10.0.0.19', agent_id='agent-tcp', agent_online=True)
        deployment = ApplicationDeployment.objects.create(host=host, instance_name='demo-tcp')
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=deployment)
        execution = InspectionExecution.objects.create(
            task_snapshot={'timeout_seconds': 10},
            group_snapshot={'scope': InspectionGroup.Scope.PER_DEPLOYMENT, 'checks': [{
                'name': 'local-port', 'executor': 'tcp', 'config': {'host': '', 'port': 8080},
            }]},
        )

        check = _agent_params(execution, deployment)['check_plan']['checks'][0]

        self.assertEqual(check['host'], '127.0.0.1')
        self.assertEqual(check['port'], 8080)

    def test_persist_results_keeps_plan_error_and_excludes_control_check(self):
        execution = InspectionExecution.objects.create()
        target = InspectionTargetExecution.objects.create(execution=execution, target_name='demo')

        from .executor import _persist_results
        _persist_results(target, [
            {'key': 'control', 'type': 'control_status', 'name': '运行状态', 'status': 'skipped'},
            {
                'key': 'check_plan', 'type': 'plan', 'name': '应用检查计划', 'status': 'error',
                'actual': 'tcp:v1', 'message': 'dj-agent 不支持检查计划要求的能力',
            },
            {'key': 'inspection:1:0', 'type': 'shell', 'name': 'version', 'status': 'pass'},
        ], {0: InspectionSeverity.CRITICAL})

        self.assertEqual(list(target.results.values_list('name', flat=True)), ['应用检查计划', 'version'])

    def test_persist_results_replaces_previous_rows(self):
        execution = InspectionExecution.objects.create()
        target = InspectionTargetExecution.objects.create(execution=execution, target_name='demo')
        from .executor import _persist_results
        payload = [{'key': 'inspection:1:0', 'type': 'shell', 'name': 'version', 'status': 'pass'}]

        _persist_results(target, payload, {0: InspectionSeverity.CRITICAL})
        _persist_results(target, payload, {0: InspectionSeverity.CRITICAL})

        self.assertEqual(target.results.count(), 1)

    def test_host_group_task_rejects_application_variables(self):
        group = self.create_group(InspectionGroup.Scope.PER_DEPLOYMENT)
        host_group = HostGroup.objects.create(name='linux')
        serializer = InspectionTaskSerializer(data={
            'name': 'invalid-host-task',
            'group': group.pk,
            'target_type': InspectionTask.TargetType.HOST_GROUP,
            'host_group': host_group.pk,
        })

        self.assertFalse(serializer.is_valid())
        self.assertIn('host_group', serializer.errors)

    def test_agent_plan_resolves_host_variables(self):
        group = self.create_host_group_check()
        host = Host.objects.create(instance_name='node-a', ip='10.0.0.10', agent_id='agent-host', agent_online=True)
        execution = InspectionExecution.objects.create(
            task_snapshot={'timeout_seconds': 10},
            group_snapshot={'scope': group.scope, 'checks': list(group.checks.values('name', 'executor', 'config'))},
        )

        check = _agent_params(execution, host=host)['check_plan']['checks'][0]

        self.assertEqual(check['command'], 'echo node-a')
        self.assertEqual(check['work_directory'], '/')
        self.assertEqual(check['expected'], 'node-a')

    @patch('inspection.service.threading.Thread')
    def test_run_api_expands_hosts_in_child_groups(self, thread_class):
        group = self.create_host_group_check()
        parent = HostGroup.objects.create(name='production')
        child = HostGroup.objects.create(name='database', parent=parent)
        parent_host = Host.objects.create(instance_name='node-parent', ip='10.0.0.11', group=parent, agent_id='agent-parent', agent_online=True)
        child_host = Host.objects.create(instance_name='node-child', ip='10.0.0.12', group=child, agent_id='agent-child', agent_online=True)
        task = InspectionTask.objects.create(
            name='host-group-task',
            group=group,
            target_type=InspectionTask.TargetType.HOST_GROUP,
            host_group=parent,
        )
        user = SysUser.objects.create(username='admin')

        response = InspectionTaskViewSet.as_view({'post': 'run'})(_run_request(task.pk, user), pk=task.pk)

        self.assertEqual(response.status_code, 200)
        execution_id = json.loads(response.content)['data']['execution_id']
        target_host_ids = set(InspectionExecution.objects.get(pk=execution_id).targets.values_list('host_id', flat=True))
        self.assertEqual(target_host_ids, {parent_host.pk, child_host.pk})
        thread_class.return_value.start.assert_called_once()

    @patch('inspection.service.threading.Thread')
    def test_run_api_expands_only_enabled_service_links(self, thread_class):
        group = self.create_group(InspectionGroup.Scope.PER_DEPLOYMENT)
        task = InspectionTask.objects.create(name='agent-task', group=group, logical_service=self.service)
        host = Host.objects.create(instance_name='node-1', ip='10.0.0.9', agent_id='agent-2', agent_online=True)
        enabled = ApplicationDeployment.objects.create(host=host, instance_name='enabled')
        disabled = ApplicationDeployment.objects.create(host=host, instance_name='disabled', enabled=False)
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=enabled, enabled=True)
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=disabled, enabled=True)
        user = SysUser.objects.create(username='admin')
        view = InspectionTaskViewSet.as_view({'post': 'run'})

        response = view(_run_request(task.pk, user), pk=task.pk)

        self.assertEqual(response.status_code, 200)
        response_data = json.loads(response.content)
        execution = InspectionExecution.objects.get(pk=response_data['data']['execution_id'])
        self.assertEqual(list(execution.targets.values_list('deployment_id', flat=True)), [enabled.pk])
        thread_class.return_value.start.assert_called_once()

    @patch('inspection.service.threading.Thread')
    def test_run_api_marks_offline_targets_failed_without_blocking_others(self, thread_class):
        group = self.create_group(InspectionGroup.Scope.PER_DEPLOYMENT)
        task = InspectionTask.objects.create(name='mixed-task', group=group, logical_service=self.service)
        online_host = Host.objects.create(instance_name='node-online', ip='10.0.0.30', agent_id='agent-online', agent_online=True)
        offline_host = Host.objects.create(instance_name='node-offline', ip='10.0.0.31', agent_id='agent-offline', agent_online=False)
        online = ApplicationDeployment.objects.create(host=online_host, instance_name='online-1')
        offline = ApplicationDeployment.objects.create(host=offline_host, instance_name='offline-1')
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=online, enabled=True)
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=offline, enabled=True)
        user = SysUser.objects.create(username='admin')
        view = InspectionTaskViewSet.as_view({'post': 'run'})

        response = view(_run_request(task.pk, user), pk=task.pk)

        # 一台离线不再拒绝整批任务，执行记录照常生成
        self.assertEqual(response.status_code, 200)
        response_data = json.loads(response.content)
        execution = InspectionExecution.objects.get(pk=response_data['data']['execution_id'])
        targets = {item.target_name: item for item in execution.targets.all()}
        self.assertEqual(targets['offline-1'].status, InspectionTargetExecution.Status.FAILED)
        self.assertFalse(targets['offline-1'].passed)
        self.assertEqual(targets['offline-1'].error_message, 'Agent 离线，未执行检查')
        self.assertEqual(targets['online-1'].status, InspectionTargetExecution.Status.PENDING)
        thread_class.return_value.start.assert_called_once()

    def test_run_api_rejects_user_without_permission(self):
        group = self.create_group(InspectionGroup.Scope.PER_DEPLOYMENT)
        task = InspectionTask.objects.create(name='guarded-task', group=group, logical_service=self.service)
        user = SysUser.objects.create(username='viewer')
        view = InspectionTaskViewSet.as_view({'post': 'run'})

        response = view(_run_request(task.pk, user), pk=task.pk)

        self.assertNotEqual(json.loads(response.content)['code'], 200)
        self.assertEqual(InspectionExecution.objects.count(), 0)

    @patch('inspection.executor.AgentChannelClient')
    def test_warning_check_failure_does_not_fail_target(self, client_class):
        group = self.create_group(InspectionGroup.Scope.PER_DEPLOYMENT)
        group.checks.update(severity=InspectionSeverity.WARNING)
        host = Host.objects.create(instance_name='node-warn', ip='10.0.0.40', agent_id='agent-warn', agent_online=True)
        deployment = ApplicationDeployment.objects.create(host=host, instance_name='warn-1')
        # 模板与版本由所属逻辑服务提供，shell 检查项解析变量时依赖这层关联。
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=deployment, enabled=True)
        execution = InspectionExecution.objects.create(
            task_snapshot={'timeout_seconds': 10, 'concurrency': 1},
            group_snapshot={
                'scope': group.scope,
                'checks': list(group.checks.values('name', 'executor', 'config', 'severity')),
            },
        )
        target = InspectionTargetExecution.objects.create(
            execution=execution, deployment=deployment, target_name='warn-1', agent_id_snapshot='agent-warn',
        )
        client_class.return_value.execute_automation.return_value = {
            'result_data': {
                'passed': False,
                'checks': [{
                    'key': f'inspection:{execution.pk}:0', 'type': 'shell', 'name': 'health',
                    'status': 'fail', 'expected': 'ok', 'actual': 'degraded',
                }],
            },
            'error_message': '',
        }

        run_inspection_execution(execution.pk)

        execution.refresh_from_db()
        target.refresh_from_db()
        self.assertEqual(target.status, InspectionTargetExecution.Status.SUCCESS)
        self.assertTrue(target.passed)
        self.assertEqual(execution.status, InspectionExecution.Status.SUCCESS)
        self.assertEqual(execution.summary['warning'], 1)
        self.assertEqual(execution.summary['failed'], 0)

    def test_inspection_alert_fires_then_resolves(self):
        from monitor.models import AlertHistory

        failing = InspectionExecution.objects.create(
            status=InspectionExecution.Status.FAILED,
            task_snapshot={'id': 1, 'name': 'nightly'},
            service_snapshot={'name': 'Demo Service', 'target_type': 'logical_service'},
        )
        InspectionTargetExecution.objects.create(
            execution=failing, target_name='demo-1',
            status=InspectionTargetExecution.Status.FAILED, error_message='Agent 离线，未执行检查',
        )

        sync_inspection_alert(failing)

        alert = AlertHistory.objects.get(source=AlertHistory.Source.INSPECTION)
        self.assertEqual(alert.state, AlertHistory.State.FIRING)
        self.assertEqual(alert.severity, InspectionSeverity.CRITICAL)

        passing = InspectionExecution.objects.create(
            status=InspectionExecution.Status.SUCCESS,
            task_snapshot={'id': 1, 'name': 'nightly'},
            service_snapshot={'name': 'Demo Service', 'target_type': 'logical_service'},
        )
        InspectionTargetExecution.objects.create(
            execution=passing, target_name='demo-1', status=InspectionTargetExecution.Status.SUCCESS, passed=True,
        )

        sync_inspection_alert(passing)

        alert.refresh_from_db()
        self.assertEqual(alert.state, AlertHistory.State.RESOLVED)
        self.assertEqual(AlertHistory.objects.filter(source=AlertHistory.Source.INSPECTION).count(), 1)

    @patch('inspection.service.threading.Thread')
    def test_scheduled_dispatch_only_runs_due_tasks(self, thread_class):
        group = self.create_group(InspectionGroup.Scope.PER_DEPLOYMENT)
        host = Host.objects.create(instance_name='node-cron', ip='10.0.0.50', agent_id='agent-cron', agent_online=True)
        deployment = ApplicationDeployment.objects.create(host=host, instance_name='cron-1')
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=deployment, enabled=True)
        task = InspectionTask.objects.create(
            name='cron-task', group=group, logical_service=self.service, cron_expression='*/5 * * * *',
        )

        # 首轮只建立时间基线，不应立刻补跑
        self.assertEqual(dispatch_scheduled_inspections(), 0)
        task.refresh_from_db()
        self.assertIsNotNone(task.next_run_time)

        InspectionTask.objects.filter(pk=task.pk).update(next_run_time=timezone.now() - timezone.timedelta(minutes=1))
        self.assertEqual(dispatch_scheduled_inspections(), 1)

        execution = InspectionExecution.objects.get(task=task)
        self.assertEqual(execution.trigger_type, InspectionExecution.TriggerType.SCHEDULED)
        thread_class.return_value.start.assert_called_once()

    def test_calculate_next_run_time_cleared_when_cron_removed(self):
        group = self.create_group(InspectionGroup.Scope.PER_DEPLOYMENT)
        task = InspectionTask.objects.create(
            name='cron-clear', group=group, logical_service=self.service, cron_expression='*/5 * * * *',
        )
        calculate_next_run_time(task)
        self.assertIsNotNone(task.next_run_time)

        task.cron_expression = ''
        calculate_next_run_time(task)

        task.refresh_from_db()
        self.assertIsNone(task.next_run_time)

    def test_fail_stale_inspection_executions_closes_pending(self):
        execution = InspectionExecution.objects.create(status=InspectionExecution.Status.PENDING)
        InspectionExecution.objects.filter(pk=execution.pk).update(
            create_time=timezone.now() - timezone.timedelta(minutes=30),
        )

        self.assertEqual(fail_stale_inspection_executions(), 1)

        execution.refresh_from_db()
        self.assertEqual(execution.status, InspectionExecution.Status.FAILED)
        self.assertIn('重启', execution.summary['error'])