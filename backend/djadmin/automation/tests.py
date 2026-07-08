from urllib.parse import unquote
from unittest.mock import patch

from django.core.files.uploadedfile import SimpleUploadedFile
from django.test import TestCase
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings

from assets.models import Host, HostGroup, HostSystem
from user.models import SysUser

from .models import AutomationTask, AutomationWorkflowRun, PlaybookTemplate


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
		AutomationTask.objects.create(
			name='Scope Task',
			code='scope-task',
			template=self.template,
			selected_host_ids=[self.host_a.id],
			selected_group_ids=[self.root_group.id],
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

	def test_task_list_empty_scope_is_all_hosts(self):
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
		self.assertEqual(summary['label'], '全部主机（2台）')
		self.assertEqual(summary['group_count'], 0)
		self.assertEqual(summary['host_count'], 2)


class AutomationRunDispatchTest(BaseTestCase):
	def setUp(self):
		super().setUp()
		self.template = PlaybookTemplate.objects.create(
			name='Dispatch Template',
			description='dispatch',
			content='---\n- hosts: all\n  tasks: []\n',
		)
		self.host = Host.objects.create(instance_name='dispatch_host', ip='10.0.0.10')
		self.task = AutomationTask.objects.create(
			name='Dispatch Task',
			code='dispatch-task',
			template=self.template,
			selected_host_ids=[self.host.id],
			selected_group_ids=[],
			env_vars={},
			enabled=True,
		)

	def test_run_template_dispatches_to_celery(self):
		with patch('automation.views.execute_ansible_job_task.delay') as mock_delay:
			res = self.client.post(
				f'/sys/automation/playbooks/{self.template.id}/run/',  # type: ignore[attr-defined]
				{'host_ids': [self.host.id], 'group_ids': [], 'extra_vars': {}},
				format='json',
			)
			body = self.assertResponseOK(res)
			self.assertEqual(body['data']['status'], 'pending')
			mock_delay.assert_called_once()

	def test_run_now_dispatches_to_celery(self):
		with patch('automation.views.execute_ansible_job_task.delay') as mock_delay:
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
		task = AutomationTask.objects.create(
			name='Dispatch All Task',
			code='dispatch-all-task',
			template=self.template,
			selected_host_ids=[],
			selected_group_ids=[],
			env_vars={},
			enabled=True,
		)

		with patch('automation.views.execute_ansible_job_task.delay') as mock_delay:
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


class AutomationWorkflowTest(BaseTestCase):
	def setUp(self):
		super().setUp()
		self.template = PlaybookTemplate.objects.create(
			name='Workflow Template',
			description='for workflow tests',
			content='---\n- hosts: all\n  tasks: []\n',
		)
		self.host = Host.objects.create(instance_name='workflow_host', ip='10.0.0.21')
		self.task = AutomationTask.objects.create(
			name='Workflow Task',
			code='workflow-task',
			template=self.template,
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

	def test_create_workflow_and_preview(self):
		workflow = self._create_workflow()

		res = self.client.post(
			f"/sys/automation/workflows/{workflow['id']}/preview/",
			{},
			format='json',
		)

		body = self.assertResponseOK(res)
		self.assertEqual(body['data']['planned_steps'], 1)
		self.assertEqual(body['data']['plan'][0]['node_key'], 'n1')

	def test_preview_workflow_with_multiple_roots(self):
		res = self.client.post(
			'/sys/automation/workflows/',
			{
				'name': '多入口工作流',
				'description': 'multi root',
				'enabled': True,
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
			f"/sys/automation/workflows/{workflow['id']}/preview/",
			{},
			format='json',
		)
		body = self.assertResponseOK(preview_res)
		self.assertEqual(body['data']['planned_steps'], 2)
		planned_keys = {item['node_key'] for item in body['data']['plan']}
		self.assertEqual(planned_keys, {'n1', 'n2'})

	def test_launch_workflow_dispatches_task_jobs(self):
		workflow = self._create_workflow()

		with patch('automation.views.execute_ansible_job_task.delay') as mock_delay:
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
