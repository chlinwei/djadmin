from typing import Any, Callable, cast

from asgiref.sync import async_to_sync
from channels.routing import URLRouter
from channels.testing import WebsocketCommunicator
from django.test import TestCase
from rest_framework_jwt.settings import api_settings

from automation.models import AnsibleExecutionJob, PlaybookTemplate
from automation.routing import websocket_urlpatterns
from user.models import SysUser


class AutomationJobLogConsumerTest(TestCase):
    def setUp(self):
        self.application = URLRouter(websocket_urlpatterns)
        self.user = SysUser.objects.create(
            username='admin',
            password='admin123',
            status=1,
            email='admin@test.com',
            timezone='Asia/Shanghai',
        )
        self.template = PlaybookTemplate.objects.create(
            name='ws-template',
            description='ws test template',
            content='---\n- hosts: all\n  tasks: []\n',
        )

    @staticmethod
    def _build_token(user: SysUser, perms=None) -> str:
        jwt_payload_handler = cast(Callable[[SysUser], Any], api_settings.JWT_PAYLOAD_HANDLER)
        jwt_encode_handler = cast(Callable[[Any], str], api_settings.JWT_ENCODE_HANDLER)
        payload = jwt_payload_handler(user)
        payload['perms'] = perms or []
        return jwt_encode_handler(payload)

    def test_connect_rejects_when_token_missing(self):
        async def scenario():
            communicator = WebsocketCommunicator(
                self.application,
                '/ws/automation/jobs/1/logs/',
            )
            connected, code = await communicator.connect()
            self.assertFalse(connected)
            self.assertEqual(code, 4401)

        async_to_sync(scenario)()

    def test_connect_rejects_when_permission_missing(self):
        async def scenario():
            token = self._build_token(self.user, perms=[])
            communicator = WebsocketCommunicator(
                self.application,
                f'/ws/automation/jobs/1/logs/?token={token}',
            )
            connected, code = await communicator.connect()
            self.assertFalse(connected)
            self.assertEqual(code, 4403)

        async_to_sync(scenario)()

    def test_connect_rejects_when_job_not_found(self):
        async def scenario():
            token = self._build_token(self.user, perms=['automation:jobs:view'])
            communicator = WebsocketCommunicator(
                self.application,
                f'/ws/automation/jobs/999999/logs/?token={token}',
            )
            connected, code = await communicator.connect()
            self.assertFalse(connected)
            self.assertEqual(code, 4404)

        async_to_sync(scenario)()

    def test_connect_sends_snapshot_status_and_completed_for_finished_job(self):
        job = AnsibleExecutionJob.objects.create(
            template=self.template,
            status=AnsibleExecutionJob.Status.SUCCESS,
            job_output='[2026-07-04 10:00:00] done\n',
            requested_user_id=self.user.id,
            requested_username=self.user.username,
        )

        async def scenario():
            token = self._build_token(self.user, perms=['automation:jobs:view'])
            communicator = WebsocketCommunicator(
                self.application,
                f'/ws/automation/jobs/{job.id}/logs/?token={token}',
            )
            connected, _ = await communicator.connect()
            self.assertTrue(connected)

            first = await communicator.receive_json_from(timeout=2)
            second = await communicator.receive_json_from(timeout=2)
            third = await communicator.receive_json_from(timeout=2)

            self.assertEqual(first['type'], 'snapshot')
            self.assertEqual(first['data']['job_id'], job.id)
            self.assertEqual(first['data']['status'], AnsibleExecutionJob.Status.SUCCESS)
            self.assertIn('done', first['data']['data'])

            self.assertEqual(second['type'], 'status')
            self.assertEqual(second['data']['status'], AnsibleExecutionJob.Status.SUCCESS)

            self.assertEqual(third['type'], 'completed')
            self.assertEqual(third['data']['job_id'], job.id)
            self.assertEqual(third['data']['status'], AnsibleExecutionJob.Status.SUCCESS)

        async_to_sync(scenario)()
