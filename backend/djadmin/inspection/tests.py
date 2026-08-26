import json
from unittest.mock import MagicMock, patch

from django.test import TransactionTestCase
from rest_framework.test import APIRequestFactory, force_authenticate

from assets.models import (
    Application,
    ApplicationDeployment,
    ApplicationDeploymentTemplate,
    ApplicationService,
    ApplicationServiceDeployment,
    ApplicationVersion,
    BusinessEnvironment,
    BusinessSystem,
    Host,
    HostGroup,
)
from user.models import SysUser

from .executor import _agent_params, run_inspection_execution
from .models import InspectionExecution, InspectionGroup, InspectionResult, InspectionTargetExecution, InspectionTask
from .serializers import InspectionGroupSerializer, InspectionTaskSerializer
from .views import InspectionTaskViewSet


class InspectionTestCase(TransactionTestCase):
    def setUp(self):
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

    def create_group(self, scope=InspectionGroup.Scope.CONTROLLER_ONCE):
        executor = 'http' if scope == InspectionGroup.Scope.CONTROLLER_ONCE else 'shell'
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

    def test_group_rejects_executor_for_wrong_scope(self):
        serializer = InspectionGroupSerializer(data={
            'name': 'invalid', 'scope': InspectionGroup.Scope.PER_DEPLOYMENT,
            'checks': [{'name': 'http', 'executor': 'http', 'config': {'url': 'http://localhost'}}],
        })
        self.assertFalse(serializer.is_valid())
        self.assertIn('checks', serializer.errors)

    @patch('inspection.executor.urllib.request.urlopen')
    def test_controller_execution_persists_structured_result(self, urlopen):
        response = MagicMock()
        response.status = 200
        urlopen.return_value.__enter__.return_value = response
        group = self.create_group()
        task = InspectionTask.objects.create(name='controller-task', group=group, logical_service=self.service)
        execution = InspectionExecution.objects.create(
            task=task,
            task_snapshot={'timeout_seconds': 10, 'concurrency': 1},
            group_snapshot={'scope': group.scope, 'checks': list(group.checks.values('name', 'executor', 'config'))},
        )
        target = InspectionTargetExecution.objects.create(execution=execution, target_name=self.service.name)

        run_inspection_execution(execution.pk)

        execution.refresh_from_db()
        target.refresh_from_db()
        self.assertEqual(execution.status, InspectionExecution.Status.SUCCESS)
        self.assertTrue(target.passed)
        self.assertEqual(InspectionResult.objects.get(target=target).status, 'pass')

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

    def test_persist_results_excludes_protocol_control_check(self):
        execution = InspectionExecution.objects.create()
        target = InspectionTargetExecution.objects.create(execution=execution, target_name='demo')

        from .executor import _persist_results
        _persist_results(target, [
            {'key': 'control', 'type': 'control_status', 'name': '运行状态', 'status': 'skipped'},
            {'key': 'inspection:1:0', 'type': 'shell', 'name': 'version', 'status': 'pass'},
        ])

        self.assertEqual(list(target.results.values_list('name', flat=True)), ['version'])

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

    @patch('inspection.views.threading.Thread')
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
        user = SysUser.objects.create(username='host-inspection-user')
        request = APIRequestFactory().post(f'/sys/inspection/tasks/{task.pk}/run/', {}, format='json')
        force_authenticate(request, user=user)

        response = InspectionTaskViewSet.as_view({'post': 'run'})(request, pk=task.pk)

        self.assertEqual(response.status_code, 200)
        execution_id = json.loads(response.content)['data']['execution_id']
        target_host_ids = set(InspectionExecution.objects.get(pk=execution_id).targets.values_list('host_id', flat=True))
        self.assertEqual(target_host_ids, {parent_host.pk, child_host.pk})
        thread_class.return_value.start.assert_called_once()

    @patch('inspection.views.threading.Thread')
    def test_run_api_expands_only_enabled_service_links(self, thread_class):
        group = self.create_group(InspectionGroup.Scope.PER_DEPLOYMENT)
        task = InspectionTask.objects.create(name='agent-task', group=group, logical_service=self.service)
        host = Host.objects.create(instance_name='node-1', ip='10.0.0.9', agent_id='agent-2', agent_online=True)
        enabled = ApplicationDeployment.objects.create(host=host, instance_name='enabled')
        disabled = ApplicationDeployment.objects.create(host=host, instance_name='disabled', enabled=False)
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=enabled, enabled=True)
        ApplicationServiceDeployment.objects.create(service=self.service, deployment=disabled, enabled=True)
        user = SysUser.objects.create(username='inspection-user')
        request = APIRequestFactory().post(f'/sys/inspection/tasks/{task.pk}/run/', {}, format='json')
        force_authenticate(request, user=user)
        view = InspectionTaskViewSet.as_view({'post': 'run'})

        response = view(request, pk=task.pk)

        self.assertEqual(response.status_code, 200)
        response_data = json.loads(response.content)
        execution = InspectionExecution.objects.get(pk=response_data['data']['execution_id'])
        self.assertEqual(list(execution.targets.values_list('deployment_id', flat=True)), [enabled.pk])
        thread_class.return_value.start.assert_called_once()