from typing import Any, Callable, cast

from django.test import TestCase
from rest_framework.test import APIClient
from rest_framework_jwt.settings import api_settings

from user.models import SysUser

from .models import AnsibleExecutionJob, PlaybookTemplate


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

    def test_job_list_supports_status_and_output_keyword_filter(self):
        matched = AnsibleExecutionJob.objects.create(
            template=self.template,
            status=AnsibleExecutionJob.Status.SUCCESS,
            requested_username='alice',
            job_output='deploy finished successfully',
        )
        AnsibleExecutionJob.objects.create(
            template=self.template,
            status=AnsibleExecutionJob.Status.SUCCESS,
            requested_username='bob',
            job_output='rollback due to timeout',
        )
        AnsibleExecutionJob.objects.create(
            template=self.template,
            status=AnsibleExecutionJob.Status.FAILED,
            requested_username='carol',
            job_output='deploy finished successfully but later failed',
        )

        res = self.client.get('/sys/automation/jobs/?status=success&output_keyword=finished')

        self.assertEqual(res.status_code, 200)
        body = res.json()
        self.assertEqual(body.get('code'), 200)
        data = body.get('data', {})
        self.assertEqual(data.get('count'), 1)
        self.assertEqual(data.get('results', [])[0]['id'], matched.id)
