import asyncio
import json
from urllib.parse import parse_qs
from django.db.models import Q
from typing import Any, Callable, cast

from channels.db import database_sync_to_async
from channels.generic.websocket import AsyncWebsocketConsumer
from rest_framework_jwt.settings import api_settings
from sys_config.models import SysConfig

from .models import AutomationExecutionJob, AutomationExecutionTargetLog, AutomationWorkflowRun
from .serializer import AutomationWorkflowRunSerializer


class AutomationJobLogConsumer(AsyncWebsocketConsumer):
    """Stream automation job unified logs to client over websocket."""

    FINAL_STATUSES = {
        AutomationExecutionJob.Status.SUCCESS,
        AutomationExecutionJob.Status.FAILED,
        AutomationExecutionJob.Status.CANCELLED,
    }
    DEFAULT_POLL_INTERVAL_SECONDS = 0.5

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
        self.row_offsets = {}
        self.last_scan_time = None
        self.last_scan_row_id = 0
        self.poll_interval_seconds = self.DEFAULT_POLL_INTERVAL_SECONDS

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
        self.poll_interval_seconds = await self._get_poll_interval_seconds(
            key='sys.automation.websocket.job_log_poll_interval_seconds',
            default_value=self.DEFAULT_POLL_INTERVAL_SECONDS,
            min_value=0.2,
            max_value=10.0,
        )

        output, status = await self._get_job_output_and_status(self.job_id)
        self.sent_length = len(output)
        self.row_offsets, self.last_scan_time, self.last_scan_row_id = await self._get_initial_row_offsets(self.job_id)
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
            changed_rows, status, next_scan_time, next_scan_row_id = await self._get_changed_rows_and_status(
                self.job_id,
                self.last_scan_time,
                self.last_scan_row_id,
            )

            delta = self._build_delta_from_changed_rows(changed_rows)
            if delta:
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

            self.last_scan_time = next_scan_time
            self.last_scan_row_id = next_scan_row_id

            if status in self.FINAL_STATUSES:
                await self._send_event('completed', {
                    'job_id': self.job_id,
                    'status': status,
                })
                await self.close(code=1000)
                return

            await asyncio.sleep(self.poll_interval_seconds)

    def _get_token_from_query_string(self):
        raw_query = self.scope.get('query_string', b'').decode('utf-8')
        query = parse_qs(raw_query)
        token = query.get('token', [''])[0]
        return token.strip()

    def _decode_token(self, token):
        try:
            decode_handler = cast(Callable[[str], dict[str, Any]], api_settings.JWT_DECODE_HANDLER)
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
        return AutomationExecutionJob.objects.filter(id=job_id).exists()

    @database_sync_to_async
    def _get_job_output_and_status(self, job_id):
        job = AutomationExecutionJob.objects.filter(id=job_id).only('status').first()
        if not job:
            return '', AutomationExecutionJob.Status.FAILED
        rows = AutomationExecutionTargetLog.objects.filter(job_id=job_id).order_by('id')
        chunks: list[str] = []
        for row in rows:
            chunks.append(
                f"\n\n===== Agent Host #{row.host_id_snapshot or '-'} ({row.host_ip_snapshot or '-'}) | status={row.status or 'unknown'} | job={row.agent_job_id or '-'} =====\n"
            )
            if row.stdout:
                chunks.append(str(row.stdout).rstrip('\n') + '\n')
            if row.stderr:
                chunks.append('[stderr]\n' + str(row.stderr).rstrip('\n') + '\n')
            if row.error_message:
                chunks.append('[error]\n' + str(row.error_message).rstrip('\n') + '\n')
        return ''.join(chunks), job.status

    @database_sync_to_async
    def _get_initial_row_offsets(self, job_id):
        rows = list(
            AutomationExecutionTargetLog.objects
            .filter(job_id=job_id)
            .order_by('update_time', 'id')
            .values('id', 'stdout', 'stderr', 'error_message', 'update_time')
        )
        offsets = {}
        for row in rows:
            row_id = int(row.get('id') or 0)
            if row_id <= 0:
                continue
            offsets[row_id] = {
                'stdout': len(str(row.get('stdout') or '')),
                'stderr': len(str(row.get('stderr') or '')),
                'error_message': len(str(row.get('error_message') or '')),
                'header_sent': True,
            }

        if not rows:
            return offsets, None, 0

        last_row = rows[-1]
        return offsets, last_row.get('update_time'), int(last_row.get('id') or 0)

    @database_sync_to_async
    def _get_changed_rows_and_status(self, job_id, last_scan_time, last_scan_row_id):
        job = AutomationExecutionJob.objects.filter(id=job_id).only('status').first()
        if not job:
            return [], AutomationExecutionJob.Status.FAILED, last_scan_time, last_scan_row_id

        queryset = AutomationExecutionTargetLog.objects.filter(job_id=job_id)
        if last_scan_time is not None:
            queryset = queryset.filter(
                Q(update_time__gt=last_scan_time)
                | (Q(update_time=last_scan_time) & Q(id__gt=int(last_scan_row_id or 0)))
            )

        rows = list(
            queryset
            .order_by('update_time', 'id')
            .values(
                'id',
                'host_id_snapshot',
                'host_ip_snapshot',
                'agent_job_id',
                'status',
                'stdout',
                'stderr',
                'error_message',
                'update_time',
            )
        )

        if not rows:
            return [], job.status, last_scan_time, last_scan_row_id

        last_row = rows[-1]
        next_scan_time = last_row.get('update_time')
        next_scan_row_id = int(last_row.get('id') or 0)
        return rows, job.status, next_scan_time, next_scan_row_id

    def _build_delta_from_changed_rows(self, rows):
        chunks: list[str] = []
        for row in rows:
            row_id = int(row.get('id') or 0)
            if row_id <= 0:
                continue

            state = self.row_offsets.get(row_id)
            if state is None:
                state = {
                    'stdout': 0,
                    'stderr': 0,
                    'error_message': 0,
                    'header_sent': False,
                }

            if not state.get('header_sent'):
                chunks.append(
                    f"\n\n===== Agent Host #{row.get('host_id_snapshot') or '-'} ({row.get('host_ip_snapshot') or '-'}) | status={row.get('status') or 'unknown'} | job={row.get('agent_job_id') or '-'} =====\n"
                )
                state['header_sent'] = True

            for field_name, prefix in (
                ('stdout', ''),
                ('stderr', '[stderr]\n'),
                ('error_message', '[error]\n'),
            ):
                current_value = str(row.get(field_name) or '')
                previous_len = int(state.get(field_name) or 0)
                current_len = len(current_value)

                if current_len < previous_len:
                    # Field was overwritten/truncated; send full content to keep client consistent.
                    previous_len = 0

                if current_len > previous_len:
                    delta_piece = current_value[previous_len:]
                    if prefix:
                        chunks.append(prefix + delta_piece.rstrip('\n') + '\n')
                    else:
                        chunks.append(delta_piece)
                    state[field_name] = current_len

            self.row_offsets[row_id] = state

        return ''.join(chunks)

    @database_sync_to_async
    def _get_poll_interval_seconds(self, key, default_value, min_value, max_value):
        cfg, _ = SysConfig.objects.get_or_create(
            key=key,
            defaults={
                'value': str(default_value),
                'default_value': str(default_value),
                'value_type': 'string',
                'name': '自动化作业日志 WS 轮询间隔（秒）',
                'description': '自动化作业日志 WebSocket 轮询后端状态的间隔（秒）',
                'is_readonly': False,
            },
        )
        try:
            parsed = float(str(cfg.value).strip())
        except (TypeError, ValueError):
            parsed = float(default_value)
        return max(float(min_value), min(float(max_value), parsed))


class AutomationWorkflowRunConsumer(AsyncWebsocketConsumer):
    """Stream workflow run status and node states to client over websocket."""

    FINAL_STATUSES = {
        AutomationWorkflowRun.Status.SUCCESS,
        AutomationWorkflowRun.Status.FAILED,
    }
    DEFAULT_POLL_INTERVAL_SECONDS = 0.5

    async def connect(self):
        route = self.scope.get('url_route')
        kwargs = route.get('kwargs') if isinstance(route, dict) else None
        run_id_value = kwargs.get('run_id') if isinstance(kwargs, dict) else None
        if run_id_value is None:
            await self.close(code=4400)
            return

        self.run_id = int(run_id_value)
        self.stream_task = None
        self.connected = False
        self.last_payload_text = ''
        self.last_run_summary = None
        self.poll_interval_seconds = self.DEFAULT_POLL_INTERVAL_SECONDS

        token = self._get_token_from_query_string()
        if not token:
            await self.close(code=4401)
            return

        payload = self._decode_token(token)
        if not payload:
            await self.close(code=4401)
            return

        if not self._has_workflow_view_permission(payload):
            await self.close(code=4403)
            return

        exists = await self._run_exists(self.run_id)
        if not exists:
            await self.close(code=4404)
            return

        await self.accept()
        self.connected = True
        self.poll_interval_seconds = await self._get_poll_interval_seconds(
            key='sys.automation.websocket.workflow_run_poll_interval_seconds',
            default_value=self.DEFAULT_POLL_INTERVAL_SECONDS,
            min_value=0.2,
            max_value=10.0,
        )

        run_data = await self._get_run_payload(self.run_id)
        if run_data is None:
            await self.close(code=4404)
            return

        self.last_run_summary = await self._get_run_summary(self.run_id)

        self.last_payload_text = json.dumps(run_data, ensure_ascii=False, sort_keys=True)
        await self._send_event('snapshot', run_data)
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
            current_summary = await self._get_run_summary(self.run_id)
            if current_summary is None:
                await self.close(code=4404)
                return

            status = str(current_summary.get('status') or '').lower()
            has_changed = current_summary != self.last_run_summary
            if has_changed:
                run_data = await self._get_run_payload(self.run_id)
                if run_data is None:
                    await self.close(code=4404)
                    return

                payload_text = json.dumps(run_data, ensure_ascii=False, sort_keys=True)
                if payload_text != self.last_payload_text:
                    self.last_payload_text = payload_text
                    await self._send_event('update', run_data)
                self.last_run_summary = current_summary

            if status in self.FINAL_STATUSES:
                run_data = await self._get_run_payload(self.run_id)
                if run_data is None:
                    await self.close(code=4404)
                    return
                await self._send_event('completed', run_data)
                await self.close(code=1000)
                return

            await asyncio.sleep(self.poll_interval_seconds)

    def _get_token_from_query_string(self):
        raw_query = self.scope.get('query_string', b'').decode('utf-8')
        query = parse_qs(raw_query)
        token = query.get('token', [''])[0]
        return token.strip()

    def _decode_token(self, token):
        try:
            decode_handler = cast(Callable[[str], dict[str, Any]], api_settings.JWT_DECODE_HANDLER)
            return decode_handler(token)
        except Exception:
            return None

    def _has_workflow_view_permission(self, payload):
        perms = payload.get('perms') or []
        return 'automation:workflow:view' in perms

    async def _send_event(self, event_type, data):
        await self.send(text_data=json.dumps({'type': event_type, 'data': data}))

    @database_sync_to_async
    def _run_exists(self, run_id):
        return AutomationWorkflowRun.objects.filter(id=run_id).exists()

    @database_sync_to_async
    def _get_run_payload(self, run_id):
        run = AutomationWorkflowRun.objects.select_related('workflow').filter(id=run_id).first()
        if not run:
            return None
        serializer = AutomationWorkflowRunSerializer(run)
        data = serializer.data
        return data if isinstance(data, dict) else None

    @database_sync_to_async
    def _get_run_summary(self, run_id):
        run = AutomationWorkflowRun.objects.filter(id=run_id).only('status', 'update_time').first()
        if not run:
            return None
        return {
            'status': str(run.status or '').lower(),
            'update_time': run.update_time,
        }

    @database_sync_to_async
    def _get_poll_interval_seconds(self, key, default_value, min_value, max_value):
        cfg, _ = SysConfig.objects.get_or_create(
            key=key,
            defaults={
                'value': str(default_value),
                'default_value': str(default_value),
                'value_type': 'string',
                'name': '工作流运行状态 WS 轮询间隔（秒）',
                'description': '工作流运行状态 WebSocket 轮询后端状态的间隔（秒）',
                'is_readonly': False,
            },
        )
        try:
            parsed = float(str(cfg.value).strip())
        except (TypeError, ValueError):
            parsed = float(default_value)
        return max(float(min_value), min(float(max_value), parsed))
