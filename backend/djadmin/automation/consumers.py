import asyncio
import json
from urllib.parse import parse_qs

from channels.db import database_sync_to_async
from channels.generic.websocket import AsyncWebsocketConsumer
from rest_framework_jwt.settings import api_settings

from .models import AnsibleExecutionJob


class AutomationJobLogConsumer(AsyncWebsocketConsumer):
    """Stream automation job unified logs to client over websocket."""

    FINAL_STATUSES = {
        AnsibleExecutionJob.Status.SUCCESS,
        AnsibleExecutionJob.Status.FAILED,
        AnsibleExecutionJob.Status.CANCELLED,
    }

    async def connect(self):
        route = self.scope.get('url_route')
        kwargs = route.get('kwargs') if isinstance(route, dict) else None
        job_id_value = kwargs.get('job_id') if isinstance(kwargs, dict) else None
        if job_id_value is None:
            await self.close(code=4400)
            return

        self.job_id = int(job_id_value)
        self.sent_length = 0
        self.stream_task = None
        self.connected = False

        token = self._get_token_from_query_string()
        if not token:
            await self.close(code=4401)
            return

        payload = self._decode_token(token)
        if not payload:
            await self.close(code=4401)
            return

        if not self._has_job_view_permission(payload):
            await self.close(code=4403)
            return

        exists = await self._job_exists(self.job_id)
        if not exists:
            await self.close(code=4404)
            return

        await self.accept()
        self.connected = True

        output, status = await self._get_job_output_and_status(self.job_id)
        self.sent_length = len(output)
        await self._send_event('snapshot', {
            'job_id': self.job_id,
            'status': status,
            'data': output,
        })

        self.stream_task = asyncio.create_task(self._stream_loop())

    async def disconnect(self, close_code):
        self.connected = False
        if self.stream_task is not None:
            self.stream_task.cancel()
            try:
                await self.stream_task
            except asyncio.CancelledError:
                pass
            self.stream_task = None

    async def _stream_loop(self):
        while self.connected:
            output, status = await self._get_job_output_and_status(self.job_id)

            if len(output) < self.sent_length:
                self.sent_length = 0

            if len(output) > self.sent_length:
                delta = output[self.sent_length:]
                self.sent_length = len(output)
                await self._send_event('output', {
                    'job_id': self.job_id,
                    'status': status,
                    'data': delta,
                })
            else:
                await self._send_event('status', {
                    'job_id': self.job_id,
                    'status': status,
                })

            if status in self.FINAL_STATUSES:
                await self._send_event('completed', {
                    'job_id': self.job_id,
                    'status': status,
                })
                await self.close(code=1000)
                return

            await asyncio.sleep(0.5)

    def _get_token_from_query_string(self):
        raw_query = self.scope.get('query_string', b'').decode('utf-8')
        query = parse_qs(raw_query)
        token = query.get('token', [''])[0]
        return token.strip()

    def _decode_token(self, token):
        try:
            decode_handler = api_settings.JWT_DECODE_HANDLER
            return decode_handler(token)
        except Exception:
            return None

    def _has_job_view_permission(self, payload):
        perms = payload.get('perms') or []
        return 'automation:jobs:view' in perms

    async def _send_event(self, event_type, data):
        await self.send(text_data=json.dumps({'type': event_type, 'data': data}))

    @database_sync_to_async
    def _job_exists(self, job_id):
        return AnsibleExecutionJob.objects.filter(id=job_id).exists()

    @database_sync_to_async
    def _get_job_output_and_status(self, job_id):
        job = AnsibleExecutionJob.objects.filter(id=job_id).only('job_output', 'status').first()
        if not job:
            return '', AnsibleExecutionJob.Status.FAILED
        return job.job_output or '', job.status
