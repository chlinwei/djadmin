from typing import Any, Callable, cast

from django.test import TestCase
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings

from user.models import SysUser

from .models import AutomationExecutionJob, AutomationExecutionTargetLog, PlaybookTemplate


def _get_token(user: SysUser) -> str:
    jwt_payload_handler = cast(Callable[[SysUser], Any], api_settings.JWT_PAYLOAD_HANDLER)
    jwt_encode_handler = cast(Callable[[Any], str], api_settings.JWT_ENCODE_HANDLER)
    payload = jwt_payload_handler(user)
    return jwt_encode_handler(payload)


class AutomationJobFilterTest(TestCase):
    def setUp(self):
        self.client = APIClient()
        self.user = SysUser.objects.create(username='admin', password='admin123', status=1)
        token = _get_token(self.user)
        self.client.credentials(HTTP_AUTHORIZATION=token)

        self.template = PlaybookTemplate.objects.create(
            name='filter-template',
            description='filter test',
            content='---\n- hosts: all\n  tasks: []\n',
        )

    def test_job_list_output_keyword_matches_target_logs(self):
        matched = AutomationExecutionJob.objects.create(
            status=AutomationExecutionJob.Status.SUCCESS,
            requested_username='alice',
            template_name_snapshot=self.template.name,
            template_content_snapshot=self.template.content,
        )
        AutomationExecutionTargetLog.objects.create(
            job=matched,
            host_id_snapshot=1,
            host_name_snapshot='host-a',
            host_ip_snapshot='10.0.0.1',
            status=AutomationExecutionTargetLog.Status.SUCCESS,
            stdout='deployment finished successfully',
        )
        non_match = AutomationExecutionJob.objects.create(
            status=AutomationExecutionJob.Status.SUCCESS,
            requested_username='bob',
            template_name_snapshot=self.template.name,
            template_content_snapshot=self.template.content,
        )
        AutomationExecutionTargetLog.objects.create(
            job=non_match,
            host_id_snapshot=2,
            host_name_snapshot='host-b',
            host_ip_snapshot='10.0.0.2',
            status=AutomationExecutionTargetLog.Status.SUCCESS,
            stdout='nothing useful here',
        )
        failed = AutomationExecutionJob.objects.create(
            status=AutomationExecutionJob.Status.FAILED,
            requested_username='carol',
            template_name_snapshot=self.template.name,
            template_content_snapshot=self.template.content,
        )
        AutomationExecutionTargetLog.objects.create(
            job=failed,
            host_id_snapshot=3,
            host_name_snapshot='host-c',
            host_ip_snapshot='10.0.0.3',
            status=AutomationExecutionTargetLog.Status.FAILED,
            stderr='finished but failed',
        )
        res = self.client.get('/sys/automation/jobs/?status=success&output_keyword=finished')

        self.assertEqual(res.status_code, 200)
        body = res.json()
        self.assertEqual(body.get('code'), 200)
        data = body.get('data', {})
        self.assertEqual(data.get('count'), 1)
        result_ids = {item['id'] for item in data.get('results', [])}
        self.assertIn(matched.id, result_ids)
        self.assertNotIn(non_match.id, result_ids)

    def test_job_list_keyword_matches_initiator_not_job_uuid(self):
        keyword = 'owner-alice'
        matched = AutomationExecutionJob.objects.create(
            status=AutomationExecutionJob.Status.SUCCESS,
            requested_username=keyword,
            template_name_snapshot=self.template.name,
            template_content_snapshot=self.template.content,
        )
        AutomationExecutionJob.objects.create(
            status=AutomationExecutionJob.Status.SUCCESS,
            requested_username='bob',
            job_id=f'{keyword}-job-id',
            template_name_snapshot=self.template.name,
            template_content_snapshot=self.template.content,
        )

        res = self.client.get(f'/sys/automation/jobs/?keyword={keyword}')

        self.assertEqual(res.status_code, 200)
        body = res.json()
        self.assertEqual(body.get('code'), 200)
        data = body.get('data', {})
        self.assertEqual(data.get('count'), 1)
        self.assertEqual(data.get('results', [])[0]['id'], matched.id)

    def test_job_list_keyword_no_longer_matches_execution_record_id(self):
        unmatched = AutomationExecutionJob.objects.create(
            status=AutomationExecutionJob.Status.SUCCESS,
            requested_username='operator',
            template_name_snapshot=self.template.name,
            template_content_snapshot=self.template.content,
        )

        res = self.client.get(f'/sys/automation/jobs/?keyword={unmatched.id}')

        self.assertEqual(res.status_code, 200)
        body = res.json()
        self.assertEqual(body.get('code'), 200)
        data = body.get('data', {})
        self.assertEqual(data.get('count'), 0)
