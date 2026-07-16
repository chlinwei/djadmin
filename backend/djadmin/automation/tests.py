from urllib.parse import unquote
from unittest.mock import patch
from datetime import timedelta

from django.core.files.uploadedfile import SimpleUploadedFile
from django.test import TestCase
from django.utils import timezone
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings

from assets.models import Host, HostGroup, HostSystem
from user.models import SysUser

from .executor import execute_ansible_job
from .models import AutomationInventory, AutomationTask, AutomationWorkflowRun, PlaybookTemplate
from .models import AnsibleExecutionJob


def _get_token(user: SysUser) -> str:
	jwt_payload_handler = api_settings.JWT_PAYLOAD_HANDLER  # type: ignore[operator]
	jwt_encode_handler = api_settings.JWT_ENCODE_HANDLER  # type: ignore[operator]
	payload = jwt_payload_handler(user)  # type: ignore[operator]
	return jwt_encode_handler(payload)  # type: ignore[operator]


class BaseTestCase(TestCase):
	def setUp(self):
		self.client = APIClient()
		self.user = SysUser.objects.create(
			username='admin', password='admin123', status=1
		)
		token = _get_token(self.user)
		self.client.credentials(HTTP_AUTHORIZATION=token)

	def assertResponseOK(self, res):
		body = res.json()
		self.assertIn('code', body)
		self.assertIn('msg', body)
		self.assertIn('data', body)
		self.assertEqual(body['code'], 200, msg=f"Expected code=200, got: {body}")
		return body


class AnsibleExecutionJobIdempotencyTest(TestCase):
	def test_running_job_is_not_reexecuted(self):
		start_time = timezone.now() - timedelta(minutes=1)
		job = AnsibleExecutionJob.objects.create(
			status=AnsibleExecutionJob.Status.RUNNING,
			start_time=start_time,
			result_summary={'message': 'already running'},
		)

		with patch('automation.executor.close_old_connections'):
			execute_ansible_job(job.id)

		job.refresh_from_db()
		self.assertEqual(job.status, AnsibleExecutionJob.Status.RUNNING)
		self.assertEqual(job.result_summary, {'message': 'already running'})
		self.assertEqual(job.start_time, start_time)

	def test_replayed_message_after_completion_is_ignored(self):
		job = AnsibleExecutionJob.objects.create(
			status=AnsibleExecutionJob.Status.PENDING,
			template_content_snapshot='',
			inventory_snapshot={'hosts': []},
		)

		with patch('automation.executor.close_old_connections'):
			execute_ansible_job(job.id)
		job.refresh_from_db()
		self.assertEqual(job.status, AnsibleExecutionJob.Status.FAILED)
		first_end_time = job.end_time

		with patch('automation.executor.close_old_connections'):
			execute_ansible_job(job.id)
		job.refresh_from_db()
		self.assertEqual(job.status, AnsibleExecutionJob.Status.FAILED)
		self.assertEqual(job.end_time, first_end_time)


class PlaybookTemplateFileTest(BaseTestCase):
	def setUp(self):
		super().setUp()
		self.template = PlaybookTemplate.objects.create(
			name='Deploy App',
			description='deploy service',
			content='---\n- hosts: all\n  tasks: []\n',
		)

	def test_upload_playbook_yaml_updates_content(self):
		upload = SimpleUploadedFile(
			'deploy.yaml',
			b'---\n- hosts: web\n  gather_facts: false\n',
			content_type='application/x-yaml',
		)

		res = self.client.post(
			f'/sys/automation/playbooks/{self.template.id}/upload/',  # type: ignore[attr-defined]
			{'file': upload},
		)

		body = self.assertResponseOK(res)
		self.template.refresh_from_db()
		self.assertEqual(self.template.content, '---\n- hosts: web\n  gather_facts: false\n')
		self.assertEqual(body['data']['content'], self.template.content)

	def test_upload_playbook_rejects_invalid_extension(self):
		upload = SimpleUploadedFile(
			'deploy.txt',
			b'---\n- hosts: web\n',
			content_type='text/plain',
		)

		res = self.client.post(
			f'/sys/automation/playbooks/{self.template.id}/upload/',  # type: ignore[attr-defined]
			{'file': upload},
		)

		body = res.json()
		self.assertEqual(body['code'], 400)
		self.assertEqual(body['msg'], 'Only .yml or .yaml files are supported')

	def test_upload_playbook_rejects_empty_file(self):
		upload = SimpleUploadedFile(
			'deploy.yml',
			b'   \n',
			content_type='application/x-yaml',
		)

		res = self.client.post(
			f'/sys/automation/playbooks/{self.template.id}/upload/',  # type: ignore[attr-defined]
			{'file': upload},
		)

		body = res.json()
		self.assertEqual(body['code'], 400)
		self.assertEqual(body['msg'], 'Template file is empty')

	def test_upload_playbook_rejects_invalid_yaml_syntax(self):
		upload = SimpleUploadedFile(
			'deploy.yml',
			b'---\n- hosts: web\n  tasks:\n    - name: bad\n      debug: "msg"\n      - invalid\n',
			content_type='application/x-yaml',
		)

		res = self.client.post(
			f'/sys/automation/playbooks/{self.template.id}/upload/',  # type: ignore[attr-defined]
			{'file': upload},
		)

		body = res.json()
		self.assertEqual(body['code'], 400)
		self.assertIn('Playbook YAML syntax error', body['msg'])

	def test_create_playbook_rejects_invalid_yaml_structure(self):
		res = self.client.post(
			'/sys/automation/playbooks/',
			{
				'name': 'Invalid Playbook',
				'description': 'invalid',
				'content': 'hosts: all\n',
			},
			format='json',
		)

		self.assertEqual(res.status_code, 400)
		body = res.json()
		self.assertEqual(body.get('code'), 600)
		self.assertIn('content', body.get('data', {}))

	def test_validate_playbook_content_success(self):
		res = self.client.post(
			'/sys/automation/playbooks/validate/',
			{
				'content': '---\n- hosts: all\n  gather_facts: false\n  tasks: []\n',
			},
			format='json',
		)

		body = self.assertResponseOK(res)
		self.assertTrue(body['data']['valid'])

	def test_validate_playbook_content_rejects_invalid_yaml(self):
		res = self.client.post(
			'/sys/automation/playbooks/validate/',
			{
				'content': '---\n- hosts: all\n  tasks:\n    - name: bad\n      debug: "x"\n      - invalid\n',
			},
			format='json',
		)

		body = res.json()
		self.assertEqual(body['code'], 400)
		self.assertIn('Playbook YAML syntax error', body['msg'])

	def test_download_playbook_returns_attachment(self):
		res = self.client.get(f'/sys/automation/playbooks/{self.template.id}/download/')  # type: ignore[attr-defined]

		self.assertEqual(res.status_code, 200)
		self.assertEqual(res.content.decode('utf-8'), self.template.content)
		content_disposition = unquote(res.headers.get('Content-Disposition', ''))
		self.assertIn('attachment', content_disposition)
		self.assertIn("Deploy-App.yml", content_disposition)


class AutomationTaskScopeTest(BaseTestCase):
	def setUp(self):
		super().setUp()
		self.template = PlaybookTemplate.objects.create(
			name='Health Check',
			description='health check',
			content='---\n- hosts: all\n  tasks: []\n',
		)
		self.root_group = HostGroup.objects.create(name='pvg4')
		self.child_group = HostGroup.objects.create(name='prd', parent=self.root_group)
		self.host_a = Host.objects.create(instance_name='test3', ip='10.25.66.177', group=self.child_group)
		self.host_b = Host.objects.create(instance_name='10.25.66.208', ip='10.25.66.208', group=self.child_group)
		HostSystem.objects.create(host=self.host_a, hostname='fidsdb')
		HostSystem.objects.create(host=self.host_b, hostname='pvg-esb4-208')
		self.inventory = AutomationInventory.objects.create(name='scope-inventory')
		self.inventory.selected_host_ids = [self.host_a.id]
		self.inventory.selected_group_ids = [self.root_group.id]
		self.inventory.save()
		AutomationTask.objects.create(
			name='Scope Task',
			code='scope-task',
			template=self.template,
			inventory=self.inventory,
			selected_host_ids=[],
			selected_group_ids=[],
			env_vars={},
			enabled=True,
		)

	def test_task_list_contains_execution_scope_summary_and_tree(self):
		res = self.client.get('/sys/automation/tasks/?page=1&page_size=10')

		body = self.assertResponseOK(res)
		record = body['data']['results'][0]
		summary = record['execution_scope_summary']
		tree = record['execution_scope_tree']

		self.assertEqual(summary['group_count'], 1)
		self.assertEqual(summary['host_count'], 2)
		self.assertEqual(summary['label'], '1组 / 2台主机')
		self.assertTrue(summary['has_tree'])
		self.assertEqual(record['selected_hosts'][0]['ip'], '10.25.66.177')
		self.assertEqual(tree[0]['title'], 'pvg4 (2)')
		self.assertEqual(tree[0]['children'][0]['title'], 'prd (2)')
		host_node = tree[0]['children'][0]['children'][0]
		self.assertEqual(host_node['search'], 'test3')

	def test_task_list_without_inventory_has_empty_scope(self):
		AutomationTask.objects.create(
			name='All Hosts Task',
			code='all-hosts-task',
			template=self.template,
			selected_host_ids=[],
			selected_group_ids=[],
			env_vars={},
			enabled=True,
		)

		res = self.client.get('/sys/automation/tasks/?search=all-hosts-task&page=1&page_size=10')

		body = self.assertResponseOK(res)
		record = body['data']['results'][0]
		summary = record['execution_scope_summary']
		self.assertEqual(summary['label'], '-')
		self.assertEqual(summary['group_count'], 0)
		self.assertEqual(summary['host_count'], 0)


class AutomationRunDispatchTest(BaseTestCase):
	def setUp(self):
		super().setUp()
		self.template = PlaybookTemplate.objects.create(
			name='Dispatch Template',
			description='dispatch',
			content='---\n- hosts: all\n  tasks: []\n',
		)
		self.host = Host.objects.create(instance_name='dispatch_host', ip='10.0.0.10')
		self.inventory = AutomationInventory.objects.create(
			name='dispatch-inventory',
		)
		self.inventory.selected_host_ids = [self.host.id]
		self.inventory.save()
		self.task = AutomationTask.objects.create(
			name='Dispatch Task',
			code='dispatch-task',
			template=self.template,
			inventory=self.inventory,
			selected_host_ids=[self.host.id],
			selected_group_ids=[],
			env_vars={},
			enabled=True,
		)

	def test_run_template_dispatches_to_celery(self):
		with patch('automation.tasks.execute_ansible_job_task.delay') as mock_delay:
			res = self.client.post(
				f'/sys/automation/playbooks/{self.template.id}/run/',  # type: ignore[attr-defined]
				{'host_ids': [self.host.id], 'group_ids': [], 'extra_vars': {}},
				format='json',
			)
			body = self.assertResponseOK(res)
			self.assertEqual(body['data']['status'], 'pending')
			mock_delay.assert_called_once()

	def test_run_now_dispatches_to_celery(self):
		with patch('automation.tasks.execute_ansible_job_task.delay') as mock_delay:
			res = self.client.post(
				f'/sys/automation/tasks/{self.task.id}/run_now/',  # type: ignore[attr-defined]
				{},
				format='json',
			)
			body = self.assertResponseOK(res)
			self.assertEqual(body['data']['status'], 'pending')
			mock_delay.assert_called_once()

	def test_run_now_with_empty_scope_defaults_to_all_hosts(self):
		host_b = Host.objects.create(instance_name='dispatch_host_b', ip='10.0.0.11')
		inventory = AutomationInventory.objects.create(
			name='all-hosts-inventory',
		)
		inventory.selected_host_ids = [self.host.id, host_b.id]
		inventory.save()
		task = AutomationTask.objects.create(
			name='Dispatch All Task',
			code='dispatch-all-task',
			template=self.template,
			inventory=inventory,
			selected_host_ids=[],
			selected_group_ids=[],
			env_vars={},
			enabled=True,
		)

		with patch('automation.tasks.execute_ansible_job_task.delay') as mock_delay:
			res = self.client.post(
				f'/sys/automation/tasks/{task.id}/run_now/',  # type: ignore[attr-defined]
				{},
				format='json',
			)
			body = self.assertResponseOK(res)
			self.assertEqual(body['data']['status'], 'pending')
			inventory_hosts = body['data']['inventory_snapshot']['hosts']
			host_ids = {item['host_id'] for item in inventory_hosts}
			self.assertIn(self.host.id, host_ids)
			self.assertIn(host_b.id, host_ids)
			mock_delay.assert_called_once()

	def test_run_now_fails_without_inventory(self):
		task = AutomationTask.objects.create(
			name='No Inventory Task',
			code='no-inventory-task',
			template=self.template,
			inventory=None,
			selected_host_ids=[self.host.id],
			selected_group_ids=[],
			env_vars={},
			enabled=True,
		)

		res = self.client.post(
			f'/sys/automation/tasks/{task.id}/run_now/',  # type: ignore[attr-defined]
			{},
			format='json',
		)

		self.assertEqual(res.status_code, 200)
		body = res.json()
		self.assertEqual(body['code'], 400)
		self.assertIn('未配置 Inventory', body['msg'])

	def test_create_task_without_inventory_succeeds(self):
		"""创建 Task 时不指定 Inventory，应成功创建"""
		res = self.client.post(
			'/sys/automation/tasks/',
			{
				'name': 'Task Without Inventory',
				'code': 'task-without-inventory',
				'template': self.template.id,
				'inventory': None,
				'selected_host_ids': [],
				'selected_group_ids': [],
				'env_vars': {},
				'enabled': True,
			},
			format='json',
		)

		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['inventory'], None)
		self.assertIsNone(body['data']['inventory'])

	def test_patch_task_remove_inventory_then_run_fails(self):
		"""编辑 Task 移除 Inventory 后，执行应失败"""
		# 先创建有 inventory 的 task
		res = self.client.post(
			'/sys/automation/tasks/',
			{
				'name': 'Task To Remove Inventory',
				'code': 'task-remove-inventory',
				'template': self.template.id,
				'inventory': self.inventory.id,
				'selected_host_ids': [self.host.id],
				'selected_group_ids': [],
				'env_vars': {},
				'enabled': True,
			},
			format='json',
		)
		task = self.assertResponseOK(res)['data']
		task_id = task['id']

		# 编辑移除 inventory
		res_patch = self.client.patch(
			f'/sys/automation/tasks/{task_id}/',
			{'inventory': None},
			format='json',
		)
		self.assertResponseOK(res_patch)

		# 执行应失败
		res_run = self.client.post(
			f'/sys/automation/tasks/{task_id}/run_now/',
			{},
			format='json',
		)

		self.assertEqual(res_run.status_code, 200)
		body = res_run.json()
		self.assertEqual(body['code'], 400)
		self.assertIn('未配置 Inventory', body['msg'])


class AutomationJobEventsApiTest(BaseTestCase):
	def test_job_events_endpoint_returns_empty_list(self):
		job = AnsibleExecutionJob.objects.create(
			status=AnsibleExecutionJob.Status.RUNNING,
		)

		res = self.client.get(f'/sys/automation/jobs/{job.id}/events/?stream=stderr&host_name=host-b')
		body = self.assertResponseOK(res)
		self.assertEqual(body['data'], [])

	def test_job_host_summary_endpoint_keeps_zero_event_counters(self):
		job = AnsibleExecutionJob.objects.create(
			status=AnsibleExecutionJob.Status.RUNNING,
			inventory_snapshot={
				'hosts': [
					{'host_name': 'host-a', 'host_ip': '10.0.0.1'},
					{'host_name': 'host-b', 'host_ip': '10.0.0.2'},
				]
			},
		)

		res = self.client.get(f'/sys/automation/jobs/{job.id}/host_summary/')
		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['count'], 2)

		items = body['data']['results']
		host_a = next(item for item in items if item['host_name'] == 'host-a')
		self.assertEqual(host_a['status'], AnsibleExecutionJob.Status.RUNNING)
		self.assertEqual(host_a['total_events'], 0)
		self.assertEqual(host_a['stdout_events'], 0)
		self.assertEqual(host_a['stderr_events'], 0)

		res_failed = self.client.get(f'/sys/automation/jobs/{job.id}/host_summary/?status=running')
		body_failed = self.assertResponseOK(res_failed)
		self.assertEqual(body_failed['data']['count'], 2)
		host_names = {item['host_name'] for item in body_failed['data']['results']}
		self.assertSetEqual(host_names, {'host-a', 'host-b'})

	def test_job_status_summary_endpoint_counts_by_target_status(self):
		job = AnsibleExecutionJob.objects.create(
			status=AnsibleExecutionJob.Status.RUNNING,
			inventory_snapshot={
				'hosts': [
					{'host_name': 'host-a', 'host_ip': '10.0.0.1'},
					{'host_name': 'host-b', 'host_ip': '10.0.0.2'},
					{'host_name': 'host-c', 'host_ip': '10.0.0.3'},
					{'host_name': 'host-d', 'host_ip': '10.0.0.4'},
				]
			},
		)

		res = self.client.get(f'/sys/automation/jobs/{job.id}/status_summary/')
		body = self.assertResponseOK(res)
		data = body['data']
		self.assertEqual(data['job_id'], job.id)
		self.assertEqual(data['job_status'], AnsibleExecutionJob.Status.RUNNING)
		self.assertEqual(data['total_hosts'], 4)
		self.assertEqual(data['finished_hosts'], 0)
		self.assertEqual(data['success'], 0)
		self.assertEqual(data['failed'], 0)
		self.assertEqual(data['unreachable'], 0)
		self.assertEqual(data['pending'], 4)
		self.assertEqual(data['running'], 0)
		self.assertEqual(data['skipped'], 0)

	def test_jobs_list_optionally_includes_status_summary(self):
		job = AnsibleExecutionJob.objects.create(
			status=AnsibleExecutionJob.Status.SUCCESS,
			inventory_snapshot={
				'hosts': [
					{'host_name': 'host-a', 'host_ip': '10.0.0.1'},
					{'host_name': 'host-b', 'host_ip': '10.0.0.2'},
				]
			},
		)

		res_plain = self.client.get(f'/sys/automation/jobs/?job_id={job.id}')
		body_plain = self.assertResponseOK(res_plain)
		plain_item = body_plain['data']['results'][0]
		self.assertNotIn('status_summary', plain_item)

		res_with_summary = self.client.get(
			f'/sys/automation/jobs/?job_id={job.id}&include_status_summary=1'
		)
		body_with_summary = self.assertResponseOK(res_with_summary)
		item = body_with_summary['data']['results'][0]
		self.assertIn('status_summary', item)
		summary = item['status_summary']
		self.assertEqual(summary['total_hosts'], 2)
		self.assertEqual(summary['finished_hosts'], 2)
		self.assertEqual(summary['success'], 2)
		self.assertEqual(summary['failed'], 0)
		self.assertEqual(summary['pending'], 0)


class AutomationWorkflowTest(BaseTestCase):
	def setUp(self):
		super().setUp()
		self.template = PlaybookTemplate.objects.create(
			name='Workflow Template',
			description='for workflow tests',
			content='---\n- hosts: all\n  tasks: []\n',
		)
		self.host = Host.objects.create(instance_name='workflow_host', ip='10.0.0.21')
		self.inventory = AutomationInventory.objects.create(
			name='workflow-inventory',
		)
		self.inventory.selected_host_ids = [self.host.id]
		self.inventory.save()
		self.task = AutomationTask.objects.create(
			name='Workflow Task',
			code='workflow-task',
			template=self.template,
			inventory=self.inventory,
			selected_host_ids=[self.host.id],
			selected_group_ids=[],
			env_vars={},
			enabled=True,
		)

	def _create_workflow(self):
		res = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': '发布工作流',
				'description': 'workflow mvp',
				'enabled': True,
				'nodes': [
					{'key': 'n1', 'name': '执行任务', 'node_type': 'task', 'task_id': self.task.id},
				],
				'edges': [],
				'default_extra_vars': {'env': 'test'},
			},
			format='json',
		)
		return self.assertResponseOK(res)['data']

	def _setup_workflow_with_inventory(self, workflow):
		"""为 workflow 添加 inventory，使其可以进行 precheck-launch/launch。"""
		res = self.client.patch(
			f"/sys/automation/workflows/{workflow['id']}/",
			{'default_inventory': self.inventory.id},
			format='json',
		)
		self.assertResponseOK(res)
		return workflow

	def test_create_workflow_draft_without_nodes(self):
		res = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': '草稿工作流',
				'description': 'draft only',
				'enabled': True,
				'nodes': [],
				'edges': [],
				'entry_node_key': '',
				'default_extra_vars': {},
			},
			format='json',
		)

		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['node_count'], 0)
		self.assertEqual(body['data']['edge_count'], 0)
		self.assertEqual(body['data']['entry_node_key'], '')

	def test_create_workflow_without_inventory_succeeds(self):
		"""创建 Workflow 时不指定 default_inventory，应成功创建"""
		res = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': 'Workflow Without Inventory',
				'description': 'workflow without inventory',
				'enabled': True,
				'nodes': [
					{'key': 'n1', 'name': '执行任务', 'node_type': 'task', 'task_id': self.task.id},
				],
				'edges': [],
				'default_extra_vars': {'env': 'test'},
			},
			format='json',
		)

		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['node_count'], 1)
		self.assertIsNone(body['data']['default_inventory'])

	def test_precheck_launch_fails_without_workflow_inventory(self):
		"""Workflow 未设置 default_inventory 时，precheck-launch 应返回错误"""
		workflow = self._create_workflow()
		# 不设置 inventory，直接做 precheck
		
		res = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/precheck-launch/",
			{},
			format='json',
		)

		body = self.assertResponseOK(res)
		self.assertFalse(body['data']['ok'])
		self.assertEqual(body['data']['status'], 'invalid_scope')
		self.assertIn('Inventory', body['data']['message'])

	def test_precheck_launch_returns_workflow_disabled_when_workflow_disabled(self):
		"""Workflow 被禁用时，precheck-launch 应返回 workflow_disabled。"""
		workflow = self._create_workflow()
		res_patch = self.client.patch(
			f"/sys/automation/workflows/{workflow['id']}/",
			{'enabled': False},
			format='json',
		)
		self.assertResponseOK(res_patch)

		res = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/precheck-launch/",
			{},
			format='json',
		)
		body = self.assertResponseOK(res)
		self.assertFalse(body['data']['ok'])
		self.assertEqual(body['data']['status'], 'workflow_disabled')

	def test_patch_workflow_remove_inventory_then_precheck_fails(self):
		"""编辑 Workflow 移除 default_inventory 后，precheck-launch 应失败"""
		workflow = self._create_workflow()
		self._setup_workflow_with_inventory(workflow)

		res_patch = self.client.patch(
			f"/sys/automation/workflows/{workflow['id']}/",
			{'default_inventory': None},
			format='json',
		)
		patch_body = self.assertResponseOK(res_patch)
		self.assertIsNone(patch_body['data']['default_inventory'])

		res = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/precheck-launch/",
			{},
			format='json',
		)

		body = self.assertResponseOK(res)
		self.assertFalse(body['data']['ok'])
		self.assertEqual(body['data']['status'], 'invalid_scope')
		self.assertIn('Inventory', body['data']['message'])

	def test_launch_fails_without_workflow_inventory(self):
		"""Workflow 未设置 default_inventory 时，launch 应返回错误"""
		workflow = self._create_workflow()
		# 不设置 inventory，直接启动
		
		res = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/launch/",
			{},
			format='json',
		)

		body = res.json()
		self.assertNotEqual(body.get('code'), 200, msg=f'Expected error, got: {body}')
		self.assertIn('Inventory', body.get('msg', ''), msg=f'Expected Inventory error, got: {body}')

	def test_patch_workflow_remove_inventory_then_launch_fails(self):
		"""编辑 Workflow 移除 default_inventory 后，launch 应失败"""
		workflow = self._create_workflow()
		self._setup_workflow_with_inventory(workflow)

		# 编辑移除 inventory
		res_patch = self.client.patch(
			f"/sys/automation/workflows/{workflow['id']}/",
			{'default_inventory': None},
			format='json',
		)
		self.assertResponseOK(res_patch)

		# launch 应失败
		res_launch = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/launch/",
			{},
			format='json',
		)

		body = res_launch.json()
		self.assertNotEqual(body.get('code'), 200, msg=f'Expected error, got: {body}')
		self.assertIn('Inventory', body.get('msg', ''), msg=f'Expected Inventory error, got: {body}')

	def test_patch_workflow_to_empty_nodes_succeeds(self):
		"""编辑 Workflow 将 nodes 改为空列表，应成功"""
		workflow = self._create_workflow()
		self._setup_workflow_with_inventory(workflow)

		res = self.client.patch(
			f"/sys/automation/workflows/{workflow['id']}/",
			{'nodes': [], 'edges': []},
			format='json',
		)

		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['node_count'], 0)
		self.assertEqual(body['data']['edge_count'], 0)

	def test_create_workflow_and_precheck_launch_without_global_inventory(self):
		workflow = self._create_workflow()
		self._setup_workflow_with_inventory(workflow)

		res = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/precheck-launch/",
			{},
			format='json',
		)

		body = self.assertResponseOK(res)
		self.assertTrue(body['data']['ok'])
		self.assertEqual(body['data']['status'], 'ok')
		self.assertEqual(body['data']['resolved_host_count'], 1)
		self.assertTrue(body['data']['use_global_scope'])

	def test_precheck_launch_with_multiple_roots_and_global_inventory(self):
		inventory = AutomationInventory.objects.create(
			name='workflow-precheck-inventory',
			selected_host_ids=[self.host.id],
			selected_group_ids=[],
			enabled=True,
		)

		res = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': '多入口工作流',
				'description': 'multi root',
				'enabled': True,
				'default_inventory': inventory.id,
				'nodes': [
					{'key': 'n1', 'name': '任务A', 'node_type': 'task', 'task_id': self.task.id},
					{'key': 'n2', 'name': '任务B', 'node_type': 'task', 'task_id': self.task.id},
				],
				'edges': [],
				'default_extra_vars': {},
			},
			format='json',
		)
		workflow = self.assertResponseOK(res)['data']

		preview_res = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/precheck-launch/",
			{},
			format='json',
		)
		body = self.assertResponseOK(preview_res)
		self.assertTrue(body['data']['ok'])
		self.assertEqual(body['data']['status'], 'ok')
		self.assertTrue(body['data']['use_global_scope'])
		self.assertEqual(body['data']['inventory_id'], inventory.id)
		self.assertEqual(body['data']['resolved_host_count'], 1)
		self.assertEqual(body['data']['matched_hosts_preview_total'], 1)

	def test_launch_workflow_dispatches_task_jobs(self):
		workflow = self._create_workflow()
		self._setup_workflow_with_inventory(workflow)

		with patch('automation.tasks.execute_ansible_job_task.delay') as mock_delay:
			res = self.client.post(
				f"/sys/automation/workflows/{workflow['id']}/launch/",
				{},
				format='json',
			)
			body = self.assertResponseOK(res)
			self.assertIn(body['data']['status'], ['running', 'success'])
			run = AutomationWorkflowRun.objects.get(id=body['data']['id'])
			self.assertGreaterEqual(len(run.node_results), 1)
			node_result = run.node_results[0]
			self.assertEqual(node_result['task_id'], self.task.id)
			self.assertEqual(node_result['task_name_snapshot'], self.task.name)
			self.assertEqual(node_result['template_name_snapshot'], self.template.name)
			self.assertTrue(node_result.get('job_id'))
			mock_delay.assert_called_once()

	def test_launch_workflow_uses_workflow_default_inventory_scope(self):
		host_b = Host.objects.create(instance_name='workflow_scope_host_b', ip='10.0.0.22')
		inventory = AutomationInventory.objects.create(
			name='workflow-scope-inventory',
			selected_host_ids=[host_b.id],
			selected_group_ids=[],
			enabled=True,
		)

		res = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': 'workflow with default inventory',
				'description': 'workflow default scope test',
				'enabled': True,
				'default_inventory': inventory.id,
				'nodes': [
					{'key': 'n1', 'name': '执行任务', 'node_type': 'task', 'task_id': self.task.id},
				],
				'edges': [],
				'default_extra_vars': {},
			},
			format='json',
		)
		workflow = self.assertResponseOK(res)['data']

		with patch('automation.tasks.execute_ansible_job_task.delay') as mock_delay:
			launch_res = self.client.post(
				f"/sys/automation/workflows/{workflow['id']}/launch/",
				{},
				format='json',
			)
			launch_body = self.assertResponseOK(launch_res)
			run = AutomationWorkflowRun.objects.get(id=launch_body['data']['id'])
			node_result = run.node_results[0]
			job_id = node_result.get('job_id')
			self.assertTrue(str(job_id).isdigit())

			detail_res = self.client.get(f'/sys/automation/jobs/{job_id}/')
			detail_body = self.assertResponseOK(detail_res)
			inventory_hosts = detail_body['data']['inventory_snapshot']['hosts']
			host_ids = {item['host_id'] for item in inventory_hosts}
			self.assertIn(host_b.id, host_ids)
			self.assertNotIn(self.host.id, host_ids)
			mock_delay.assert_called_once()

	def test_workflow_can_be_edited_with_circular_reference(self):
		"""Test that workflows can be edited to contain circular references (validation deferred to runtime)."""
		# 创建工作流 A
		res1 = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': 'workflow A for edit cycle test',
				'description': 'workflow A',
				'enabled': True,
				'nodes': [
					{'key': 'n1', 'name': 'task in A', 'node_type': 'task', 'task_id': self.task.id}
				],
				'edges': [],
				'default_extra_vars': {},
			},
			format='json',
		)
		workflow_a = self.assertResponseOK(res1)['data']

		# 创建工作流 B
		res2 = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': 'workflow B for edit cycle test',
				'description': 'workflow B',
				'enabled': True,
				'nodes': [
					{'key': 'n1', 'name': 'task in B', 'node_type': 'task', 'task_id': self.task.id}
				],
				'edges': [],
				'default_extra_vars': {},
			},
			format='json',
		)
		workflow_b = self.assertResponseOK(res2)['data']

		# 编辑 A 引用 B（应该成功 - 编辑时不检测循环）
		res3 = self.client.patch(
			f'/sys/automation/workflows/{workflow_a["id"]}/',
			{
				'nodes': [
					{'key': 'n1', 'name': 'task in A', 'node_type': 'task', 'task_id': self.task.id},
					{'key': 'n2', 'name': 'ref to B', 'node_type': 'workflow', 'workflow_id': workflow_b['id']}
				],
			},
			format='json',
		)
		body = self.assertResponseOK(res3)
		self.assertEqual(res3.status_code, 200)

		# 编辑 B 引用 A，形成循环（应该成功 - 编辑时不检测循环）
		res4 = self.client.patch(
			f'/sys/automation/workflows/{workflow_b["id"]}/',
			{
				'nodes': [
					{'key': 'n1', 'name': 'task in B', 'node_type': 'task', 'task_id': self.task.id},
					{'key': 'n2', 'name': 'ref to A', 'node_type': 'workflow', 'workflow_id': workflow_a['id']}
				],
			},
			format='json',
		)
		body = self.assertResponseOK(res4)
		self.assertEqual(res4.status_code, 200)
		# 验证编辑成功，工作流现在有 2 个节点
		self.assertEqual(body['data']['node_count'], 2)

	def test_workflow_with_valid_nested_workflow_node(self):
		"""Test that valid workflow nesting (without cycles) works in edit and at runtime."""
		res1 = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': 'workflow B - referenceable',
				'description': 'simple workflow to be referenced',
				'enabled': True,
				'nodes': [
					{'key': 'n1', 'name': 'task in B', 'node_type': 'task', 'task_id': self.task.id}
				],
				'edges': [],
				'default_extra_vars': {},
			},
			format='json',
		)
		workflow_b = self.assertResponseOK(res1)['data']

		res2 = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': 'workflow A - container',
				'description': 'workflow containing a task and a reference to B',
				'enabled': True,
				'nodes': [
					{'key': 'n1', 'name': 'task in A', 'node_type': 'task', 'task_id': self.task.id},
					{'key': 'n2', 'name': 'ref to B', 'node_type': 'workflow', 'workflow_id': workflow_b['id']}
				],
				'edges': [],
				'default_extra_vars': {},
			},
			format='json',
		)
		body = self.assertResponseOK(res2)
		self.assertEqual(res2.status_code, 200)
		self.assertEqual(body['data']['node_count'], 2)

	def test_workflow_cycle_detected_at_runtime(self):
		"""Test that workflow cycles are detected when executing workflow nodes.
		
		This test verifies that:
		1. Launching a workflow with cycle succeeds initially (status='running')
		2. When the workflow node is dispatched, cycle is detected
		3. The workflow node fails with cycle error message
		4. The workflow itself becomes failed
		5. Error message is visible in API response
		"""
		# 创建工作流 B - 稍后会被 A 引用
		res_b = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': 'workflow B',
				'description': 'workflow B',
				'enabled': True,
				'nodes': [
					{'key': 'nb1', 'name': 'task in B', 'node_type': 'task', 'task_id': self.task.id}
				],
				'edges': [],
				'default_inventory': self.inventory.id,
				'default_extra_vars': {},
			},
			format='json',
		)
		workflow_b = self.assertResponseOK(res_b)['data']
		workflow_b_id = workflow_b['id']

		# 创建工作流 A，引用 B
		res_a = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': 'workflow A',
				'description': 'workflow A references B',
				'enabled': True,
				'nodes': [
					{'key': 'na1', 'name': 'ref to B', 'node_type': 'workflow', 'workflow_id': workflow_b_id}
				],
				'edges': [],
				'default_inventory': self.inventory.id,
				'default_extra_vars': {},
			},
			format='json',
		)
		workflow_a = self.assertResponseOK(res_a)['data']
		workflow_a_id = workflow_a['id']

		# 编辑 B，引用 A，形成循环 A -> B -> A
		res_patch_b = self.client.patch(
			f'/sys/automation/workflows/{workflow_b_id}/',
			{
				'nodes': [
					{'key': 'nb1', 'name': 'task in B', 'node_type': 'task', 'task_id': self.task.id},
					{'key': 'nb2', 'name': 'ref to A', 'node_type': 'workflow', 'workflow_id': workflow_a_id}
				],
				'edges': [
					{'source_key': 'nb1', 'target_key': 'nb2', 'condition': 'success'}
				],
			},
			format='json',
		)
		self.assertResponseOK(res_patch_b)

		# 启动工作流 A
		res_launch = self.client.post(
			f'/sys/automation/workflows/{workflow_a_id}/launch/',
			{},
			format='json',
		)
		launch_body = self.assertResponseOK(res_launch)
		run_id = launch_body['data']['id']
		
		# 启动应该成功，状态可能是 running 或 failed（如果立即检测到循环）
		self.assertEqual(res_launch.status_code, 200)
		self.assertIn(launch_body['data']['status'], {'running', 'pending', 'failed'})

		# 获取运行状态
		res_detail = self.client.get(
			f'/sys/automation/workflow-runs/{run_id}/',
			format='json',
		)
		detail_body = self.assertResponseOK(res_detail)
		node_results = detail_body['data'].get('node_results_runtime', [])
		
		# 验证包含循环检测的错误消息
		self.assertTrue(len(node_results) > 0, f'Expected node_results_runtime, got: {node_results}')
		
		cycle_error_found = False
		for node in node_results:
			node_key = node.get('node_key', '')
			node_status = node.get('status', '')
			message = node.get('message', '')
			
			# 派发到 B 时应该检测到循环
			if node_key == 'na1' and node_status == 'failed':
				error_msg_lower = str(message).lower()
				# 检查是否包含循环检测相关的关键词
				if 'circular' in error_msg_lower or 'cycle' in error_msg_lower:
					cycle_error_found = True
					print(f'✓ Cycle detected with error message: {message}')
		
		self.assertTrue(cycle_error_found, 
			f'Expected cycle error message in node. Nodes: {node_results}')

	# ------------------------------------------------------------------ #
	# Task-disabled validation: precheck-launch / launch 均应被拦截          #
	# ------------------------------------------------------------------ #

	def test_precheck_launch_blocked_when_task_node_is_disabled(self):
		"""precheck-launch 应在节点任务被禁用时返回 ok=False, status=node_task_invalid。"""
		workflow = self._create_workflow()
		self._setup_workflow_with_inventory(workflow)

		# 禁用被引用的任务
		self.task.enabled = False
		self.task.save()

		res = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/precheck-launch/",
			{},
			format='json',
		)
		body = self.assertResponseOK(res)
		self.assertFalse(body['data']['ok'])
		self.assertEqual(body['data']['status'], 'node_task_invalid')
		self.assertIn(self.task.name, body['data']['message'])

	def test_launch_blocked_when_task_node_is_disabled(self):
		"""launch 应在节点任务被禁用时返回业务错误，不创建 WorkflowRun。"""
		workflow = self._create_workflow()
		self._setup_workflow_with_inventory(workflow)
		run_count_before = AutomationWorkflowRun.objects.count()

		self.task.enabled = False
		self.task.save()

		res = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/launch/",
			{},
			format='json',
		)
		body = res.json()
		# 应返回非 200 业务错误（code != 200 或 data 包含错误描述）
		self.assertNotEqual(body.get('code'), 200, msg=f'Expected error, got: {body}')
		# 不应创建新的 WorkflowRun
		self.assertEqual(AutomationWorkflowRun.objects.count(), run_count_before)

	def test_precheck_launch_blocked_when_task_inventory_is_disabled(self):
		"""precheck-launch 应在节点任务绑定的 Inventory 被禁用时返回 ok=False。"""
		inventory = AutomationInventory.objects.create(
			name='task-bound-inventory',
			selected_host_ids=[self.host.id],
			selected_group_ids=[],
			enabled=True,
		)
		self.task.inventory = inventory
		self.task.save()

		workflow = self._create_workflow()
		self._setup_workflow_with_inventory(workflow)

		# 禁用任务绑定的 Inventory
		inventory.enabled = False
		inventory.save()

		res = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/precheck-launch/",
			{},
			format='json',
		)
		body = self.assertResponseOK(res)
		self.assertFalse(body['data']['ok'])
		self.assertEqual(body['data']['status'], 'node_task_invalid')
		self.assertIn(inventory.name, body['data']['message'])

	def test_precheck_launch_succeeds_after_re_enabling_task(self):
		"""任务禁用后再重新启用，precheck-launch 应恢复为 ok=True。"""
		workflow = self._create_workflow()
		self._setup_workflow_with_inventory(workflow)

		self.task.enabled = False
		self.task.save()

		# 禁用时应被拦截
		res1 = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/precheck-launch/",
			{},
			format='json',
		)
		self.assertFalse(self.assertResponseOK(res1)['data']['ok'])

		# 重新启用
		self.task.enabled = True
		self.task.save()

		res2 = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/precheck-launch/",
			{},
			format='json',
		)
		self.assertTrue(self.assertResponseOK(res2)['data']['ok'])

	def test_precheck_launch_not_blocked_by_disabled_task_in_other_workflow(self):
		"""禁用某个任务只影响引用该任务的 Workflow，不影响未引用该任务的 Workflow。"""
		# 创建另一个任务（始终保持启用）
		other_task = AutomationTask.objects.create(
			name='Other Task Not Disabled',
			code='other-task-not-disabled',
			template=self.template,
			inventory=self.inventory,
			selected_host_ids=[self.host.id],
			selected_group_ids=[],
			env_vars={},
			enabled=True,
		)

		# 创建只引用 other_task 的 Workflow
		res = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': '使用其他任务的工作流',
				'description': 'uses other_task only',
				'enabled': True,
				'nodes': [
					{'key': 'n1', 'name': '其他任务节点', 'node_type': 'task', 'task_id': other_task.id},
				],
				'edges': [],
				'default_inventory': self.inventory.id,
				'default_extra_vars': {},
			},
			format='json',
		)
		other_workflow = self.assertResponseOK(res)['data']

		# 禁用 self.task（只影响引用它的 Workflow）
		self.task.enabled = False
		self.task.save()

		# other_workflow 不引用 self.task，预检应通过
		res2 = self.client.post(
			f"/sys/automation/workflows/{other_workflow['id']}/precheck-launch/",
			{},
			format='json',
		)
		body = self.assertResponseOK(res2)
		self.assertTrue(body['data']['ok'],
			msg=f'Expected ok=True for workflow not using disabled task, got: {body["data"]}')

	def test_delete_workflow_without_runs(self):
		"""测试删除不含运行记录的 Workflow"""
		workflow = self._create_workflow()
		workflow_id = workflow['id']

		res = self.client.delete(f'/sys/automation/workflows/{workflow_id}/')

		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['id'], workflow_id)
		# 验证已删除
		from .models import AutomationWorkflowTemplate
		self.assertFalse(AutomationWorkflowTemplate.objects.filter(id=workflow_id).exists())

	def test_delete_workflow_with_runs(self):
		"""测试删除包含运行记录的 Workflow - workflow 删除后，runs 保留但 workflow_id 设为 NULL"""
		workflow = self._create_workflow()
		self._setup_workflow_with_inventory(workflow)
		workflow_id = workflow['id']

		# 创建一个运行记录
		with patch('automation.tasks.execute_ansible_job_task.delay'):
			res = self.client.post(
				f"/sys/automation/workflows/{workflow_id}/launch/",
				{},
				format='json',
			)
			self.assertResponseOK(res)

		# 验证运行记录已创建
		run_count = AutomationWorkflowRun.objects.filter(workflow_id=workflow_id).count()
		self.assertGreater(run_count, 0)

		# 删除 Workflow
		res = self.client.delete(f'/sys/automation/workflows/{workflow_id}/')
		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['id'], workflow_id)

		# 验证 Workflow 已删除
		from .models import AutomationWorkflowTemplate
		self.assertFalse(AutomationWorkflowTemplate.objects.filter(id=workflow_id).exists())
		
		# 验证运行记录保留，但 workflow_id 已设为 NULL
		self.assertEqual(AutomationWorkflowRun.objects.filter(workflow_id=workflow_id).count(), 0, "运行记录的 workflow_id 已设为 NULL")
		# 验证运行记录本身仍然存在（workflow_id 为 NULL）
		orphaned_runs = AutomationWorkflowRun.objects.filter(workflow_id__isnull=True).count()
		self.assertGreater(orphaned_runs, 0, "运行记录应保留但成为孤立记录")

	def test_delete_workflow_referenced_by_other_workflow(self):
		"""测试删除被其他 Workflow 引用的 Workflow - 应拒绝删除"""
		from .models import AutomationWorkflowTemplate
		
		# 创建 Workflow B（被引用的 Workflow）
		workflow_b = self._create_workflow()
		workflow_b_id = workflow_b['id']

		# 创建 Workflow A，引用 Workflow B
		res_a = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': 'Workflow A (references B)',
				'description': 'workflow A references B',
				'enabled': True,
				'nodes': [
					{'key': 'n1', 'name': 'ref to B', 'node_type': 'workflow', 'workflow_id': workflow_b_id}
				],
				'edges': [],
				'default_extra_vars': {},
			},
			format='json',
		)
		workflow_a = self.assertResponseOK(res_a)['data']

		# 尝试删除 Workflow B（应被拒绝）
		res = self.client.delete(f'/sys/automation/workflows/{workflow_b_id}/')
		
		# 应返回 400+ 错误
		self.assertGreaterEqual(res.status_code, 400)
		body = res.json()
		self.assertNotEqual(body['code'], 200)
		self.assertIn('引用', body['msg'])  # 错误信息应提及"引用"
		
		# 验证 Workflow B 仍然存在
		self.assertTrue(AutomationWorkflowTemplate.objects.filter(id=workflow_b_id).exists())

	def test_run_record_name_preserved_after_workflow_deleted(self):
		"""测试删除 Workflow 后，运行记录的名称仍然可以从快照读取"""
		workflow = self._create_workflow()
		workflow_id = workflow['id']
		workflow_name = workflow['name']

		# 创建运行记录（写入 workflow_name_snapshot）
		run = AutomationWorkflowRun.objects.create(
			workflow_id=workflow_id,
			status='success',
			workflow_name_snapshot=workflow_name,
		)

		# 删除 Workflow
		res = self.client.delete(f'/sys/automation/workflows/{workflow_id}/')
		self.assertResponseOK(res)

		# 查询运行记录列表，名称应仍然返回快照值
		res = self.client.get('/sys/automation/workflow-runs/')
		body = self.assertResponseOK(res)
		results = body['data']['results']
		matched = [r for r in results if r['id'] == run.id]
		self.assertEqual(len(matched), 1, "运行记录应仍然存在")
		self.assertEqual(matched[0]['workflow_name'], workflow_name, "删除 workflow 后名称应从快照读取")
		self.assertIsNone(matched[0].get('workflow'), "workflow_id 应已为 NULL")

	def test_run_records_searchable_by_name_after_workflow_deleted(self):
		"""测试删除 Workflow 后，仍可通过名称搜索到运行记录"""
		workflow = self._create_workflow()
		workflow_id = workflow['id']
		workflow_name = workflow['name']

		run = AutomationWorkflowRun.objects.create(
			workflow_id=workflow_id,
			status='success',
			workflow_name_snapshot=workflow_name,
		)

		# 删除 Workflow
		res = self.client.delete(f'/sys/automation/workflows/{workflow_id}/')
		self.assertResponseOK(res)

		# 用 workflow 名称搜索，应能找到历史记录
		res = self.client.get(f'/sys/automation/workflow-runs/?search={workflow_name}')
		body = self.assertResponseOK(res)
		results = body['data']['results']
		matched = [r for r in results if r['id'] == run.id]
		self.assertEqual(len(matched), 1, "删除 workflow 后应仍能通过名称快照搜索到运行记录")

	# ──────────── 增删改查基础覆盖 ────────────

	def test_list_workflows(self):
		"""列表接口返回分页结果，已创建的 workflow 出现在结果中"""
		self._create_workflow()
		res = self.client.get('/sys/automation/workflows/')
		body = self.assertResponseOK(res)
		data = body['data']
		self.assertIn('results', data)
		self.assertGreater(data['count'], 0)
		names = [w['name'] for w in data['results']]
		self.assertIn('发布工作流', names)

	def test_retrieve_workflow(self):
		"""详情接口返回单条 workflow 数据"""
		workflow = self._create_workflow()
		res = self.client.get(f"/sys/automation/workflows/{workflow['id']}/")
		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['id'], workflow['id'])
		self.assertEqual(body['data']['name'], workflow['name'])

	def test_create_workflow_with_nodes_and_edges(self):
		"""创建包含节点和边的 workflow，字段正确保存"""
		res = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': '完整工作流',
				'description': '含节点和边',
				'enabled': True,
				'nodes': [
					{'key': 'n1', 'name': '任务A', 'node_type': 'task', 'task_id': self.task.id},
					{'key': 'n2', 'name': '任务B', 'node_type': 'task', 'task_id': self.task.id},
				],
				'edges': [
					{'source_key': 'n1', 'target_key': 'n2', 'condition': 'success'},
				],
				'default_extra_vars': {'env': 'prod'},
			},
			format='json',
		)
		body = self.assertResponseOK(res)
		data = body['data']
		self.assertEqual(data['name'], '完整工作流')
		self.assertEqual(data['node_count'], 2)
		self.assertEqual(data['edge_count'], 1)

	def test_create_workflow_duplicate_name_rejected(self):
		"""重复名称的 workflow 应被拒绝"""
		self._create_workflow()
		res = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': '发布工作流',  # 与 _create_workflow 同名
				'description': '重复',
				'enabled': True,
				'nodes': [],
				'edges': [],
				'default_extra_vars': {},
			},
			format='json',
		)
		self.assertGreaterEqual(res.status_code, 400)

	def test_update_workflow_name_and_description(self):
		"""PUT 全量更新名称和描述"""
		workflow = self._create_workflow()
		wid = workflow['id']
		res = self.client.put(
			f'/sys/automation/workflows/{wid}/',
			{
				'name': '更名后的工作流',
				'description': '已更新描述',
				'enabled': True,
				'nodes': workflow['nodes'],
				'edges': workflow['edges'],
				'default_extra_vars': {},
			},
			format='json',
		)
		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['name'], '更名后的工作流')
		self.assertEqual(body['data']['description'], '已更新描述')

	def test_partial_update_workflow_enabled(self):
		"""PATCH 部分更新 enabled 字段"""
		workflow = self._create_workflow()
		wid = workflow['id']
		res = self.client.patch(
			f'/sys/automation/workflows/{wid}/',
			{'enabled': False},
			format='json',
		)
		body = self.assertResponseOK(res)
		self.assertFalse(body['data']['enabled'])

	def test_update_workflow_nodes(self):
		"""PUT 更新节点列表，node_count 同步变更"""
		workflow = self._create_workflow()
		wid = workflow['id']
		res = self.client.put(
			f'/sys/automation/workflows/{wid}/',
			{
				'name': workflow['name'],
				'description': workflow['description'],
				'enabled': True,
				'nodes': [
					{'key': 'n1', 'name': '任务1', 'node_type': 'task', 'task_id': self.task.id},
					{'key': 'n2', 'name': '任务2', 'node_type': 'task', 'task_id': self.task.id},
					{'key': 'n3', 'name': '任务3', 'node_type': 'task', 'task_id': self.task.id},
				],
				'edges': [],
				'default_extra_vars': {},
			},
			format='json',
		)
		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['node_count'], 3)

	def test_list_workflow_runs(self):
		"""运行记录列表接口返回分页数据"""
		workflow = self._create_workflow()
		AutomationWorkflowRun.objects.create(
			workflow_id=workflow['id'],
			status='success',
			workflow_name_snapshot=workflow['name'],
		)
		res = self.client.get('/sys/automation/workflow-runs/')
		body = self.assertResponseOK(res)
		data = body['data']
		self.assertIn('results', data)
		self.assertGreater(data['count'], 0)

	def test_list_workflow_runs_filter_by_status(self):
		"""运行记录列表支持按 status 过滤"""
		workflow = self._create_workflow()
		AutomationWorkflowRun.objects.create(
			workflow_id=workflow['id'], status='success',
			workflow_name_snapshot=workflow['name'],
		)
		AutomationWorkflowRun.objects.create(
			workflow_id=workflow['id'], status='failed',
			workflow_name_snapshot=workflow['name'],
		)
		res = self.client.get('/sys/automation/workflow-runs/?status=success')
		body = self.assertResponseOK(res)
		for item in body['data']['results']:
			self.assertEqual(item['status'], 'success')

	def test_retrieve_workflow_run(self):
		"""运行记录详情接口返回单条数据"""
		workflow = self._create_workflow()
		run = AutomationWorkflowRun.objects.create(
			workflow_id=workflow['id'],
			status='running',
			workflow_name_snapshot=workflow['name'],
		)
		res = self.client.get(f'/sys/automation/workflow-runs/{run.id}/')
		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['id'], run.id)
		self.assertEqual(body['data']['workflow_name'], workflow['name'])

	def test_workflow_run_name_from_snapshot(self):
		"""运行记录的 workflow_name 始终取自 workflow_name_snapshot"""
		workflow = self._create_workflow()
		run = AutomationWorkflowRun.objects.create(
			workflow_id=workflow['id'],
			status='success',
			workflow_name_snapshot='快照名称',  # 与实际 workflow 名称不同
		)
		res = self.client.get(f'/sys/automation/workflow-runs/{run.id}/')
		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['workflow_name'], '快照名称')

	def test_list_workflow_runs_should_not_recalculate_duration_for_finished_run(self):
		"""已完成运行记录在列表刷新时不应持续重算耗时。"""
		workflow = self._create_workflow()
		start_at = timezone.now() - timedelta(hours=2)
		end_at = start_at + timedelta(minutes=5)
		run = AutomationWorkflowRun.objects.create(
			workflow_id=workflow['id'],
			status='failed',
			workflow_name_snapshot=workflow['name'],
			start_time=start_at,
			end_time=end_at,
			duration_seconds=300.0,
		)

		# 触发 list 内部的 _refresh_workflow_run_progress
		res = self.client.get('/sys/automation/workflow-runs/')
		self.assertResponseOK(res)

		run.refresh_from_db()
		self.assertEqual(run.duration_seconds, 300.0)
		self.assertEqual(run.end_time, end_at)


class JobTimeoutDetectionTest(TestCase):
	"""测试job超时自动失败检测"""

	def test_pending_job_timeout_after_5_minutes(self):
		"""Pending超过5分钟应自动标记为失败"""
		from automation.tasks import check_and_fail_stale_jobs

		# 创建一个5分钟前就处于pending的job
		job = AnsibleExecutionJob.objects.create(
			status=AnsibleExecutionJob.Status.PENDING,
			result_summary={'message': 'waiting'},
		)
		old_time = timezone.now() - timedelta(minutes=6)
		AnsibleExecutionJob.objects.filter(id=job.id).update(create_time=old_time)

		# 运行检测任务
		result = check_and_fail_stale_jobs()

		# 验证job被标记为失败
		job.refresh_from_db()
		self.assertEqual(job.status, AnsibleExecutionJob.Status.FAILED)
		self.assertIn('worker_unavailable', job.result_summary.get('fail_reason', ''))
		self.assertEqual(result['pending_failed'], 1)

	def test_running_job_timeout_with_task_timeout(self):
		"""Running超过task的timeout应自动标记为失败"""
		from automation.tasks import check_and_fail_stale_jobs

		# 创建playbook和task
		playbook = PlaybookTemplate.objects.create(
			name='test-pb',
			content='---\n- hosts: all\n  tasks:\n    - debug: msg=hello',
		)
		task = AutomationTask.objects.create(
			name='test-task',
			code='test-task',
			template=playbook,
			execution_timeout_seconds=300,  # 5分钟超时
		)

		# 创建一个10分钟前就处于running的job
		job = AnsibleExecutionJob.objects.create(
			status=AnsibleExecutionJob.Status.RUNNING,
			task=task,
			result_summary={'message': 'running...'},
		)
		old_time = timezone.now() - timedelta(minutes=11)
		AnsibleExecutionJob.objects.filter(id=job.id).update(create_time=old_time)

		# 运行检测任务
		result = check_and_fail_stale_jobs()

		# 验证job被标记为失败
		job.refresh_from_db()
		self.assertEqual(job.status, AnsibleExecutionJob.Status.FAILED)
		self.assertIn('execution_timeout', job.result_summary.get('fail_reason', ''))
		self.assertEqual(job.result_summary.get('timeout_seconds'), 300)
		self.assertEqual(result['running_failed'], 1)

	def test_recent_jobs_not_failed_by_timeout(self):
		"""最近创建的job不应被标记为超时失败"""
		from automation.tasks import check_and_fail_stale_jobs

		# 创建一个最近1分钟创建的pending job
		job = AnsibleExecutionJob.objects.create(
			status=AnsibleExecutionJob.Status.PENDING,
			result_summary={'message': 'just created'},
		)

		# 运行检测任务
		result = check_and_fail_stale_jobs()

		# 验证job仍然是pending
		job.refresh_from_db()
		self.assertEqual(job.status, AnsibleExecutionJob.Status.PENDING)
		self.assertEqual(result['pending_failed'], 0)
		self.assertEqual(result['running_failed'], 0)
