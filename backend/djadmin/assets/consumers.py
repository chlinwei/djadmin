import asyncio
import io
import json
import socket
import time
import uuid
from datetime import timedelta
from typing import Any, Callable, cast
from urllib.parse import parse_qs

from channels.db import database_sync_to_async
from channels.generic.websocket import AsyncWebsocketConsumer
from django.utils import timezone
from rest_framework_jwt.settings import api_settings

from .models import Host, HostCredential, WebSSHSessionLog
from .webssh_runtime import WebSSHRuntimeRegistry
from sys_config.models import SysConfig

try:
    import paramiko
except ImportError:  # pragma: no cover
    paramiko = None


class HostWebSSHConsumer(AsyncWebsocketConsumer):
    """Host Web SSH consumer (single-machine dev MVP)."""

    async def connect(self):
        route = self.scope.get('url_route')
        kwargs = route.get('kwargs') if isinstance(route, dict) else None
        host_id_value = kwargs.get('host_id') if isinstance(kwargs, dict) else None
        if host_id_value is None:
            await self.close(code=4400)
            return

        self.host_id = int(host_id_value)
        self.ssh_client = None
        self.ssh_channel = None
        self.reader_task = None
        self.connected = False
        self._init_audit_runtime_state()

        token = self._get_token_from_query_string()
        if not token:
            await self.close(code=4401)
            return

        payload = self._decode_token(token)
        if not payload:
            await self.close(code=4401)
            return

        if not self._has_webssh_permission(payload):
            await self.close(code=4403)
            return

        self.audit_user_id = payload.get('user_id')
        self.audit_username = payload.get('username') or ''
        self.audit_client_ip = self._get_client_ip()
        self.audit_user_agent = self._get_header('user-agent')

        host, credential, host_display_name, error_msg = await self._get_host_and_default_credential(self.host_id)
        if error_msg:
            await self.accept()
            await self._send_event('error', {'message': error_msg})
            await self.close(code=4404)
            return
        if host is None or credential is None:
            await self.close(code=4404)
            return

        await self.accept()

        self.audit_retention_days, self.audit_max_content_bytes = await self._load_webssh_audit_configs()
        await self._cleanup_expired_session_logs(self.audit_retention_days)
        self.audit_session_pk, self.audit_started_at = await self._create_session_log()

        ok, error = await self._open_ssh(host, credential)
        if not ok:
            await self._close_session_log(
                close_code=4500,
                status=WebSSHSessionLog.Status.FAILED,
                error_message=error,
            )
            await self._send_event('error', {'message': error})
            await self.close(code=4500)
            return

        self.connected = True
        WebSSHRuntimeRegistry.mark_active(self.audit_session_id, self)
        await self._send_event('connected', {
            'host_id': self.host_id,
            'host_name': host_display_name,
            'ip': host.ip,
            'session_id': self.audit_session_id,
        })

        # Trigger initial shell prompt for servers that wait for first input.
        if self.ssh_channel is not None:
            await asyncio.to_thread(self.ssh_channel.send, '\n')

        self.reader_task = asyncio.create_task(self._reader_loop())

    def _init_audit_runtime_state(self):
        self.audit_user_id = None
        self.audit_username = ''
        self.audit_client_ip = ''
        self.audit_user_agent = ''
        self.audit_session_id = str(uuid.uuid4())
        self.audit_session_pk = None
        self.audit_started_at = None
        self.audit_input_bytes = 0
        self.audit_command_count = 0
        self.audit_closed = False
        self.audit_input_content_chunks = []
        self.audit_output_content_chunks = []
        self.audit_recorded_content_bytes = 0
        self.audit_is_content_truncated = False
        self.audit_max_content_bytes = 20 * 1024 * 1024
        self.audit_retention_days = 30
        self.audit_flush_threshold_bytes = 8 * 1024
        self.audit_buffered_bytes = 0
        self.audit_stats_dirty = False
        self.audit_live_sync_interval_seconds = 1.0
        self.audit_last_sync_monotonic = time.monotonic()
        self.audit_flush_lock = asyncio.Lock()
        self.audit_close_notified = False

    async def receive(self, text_data=None, bytes_data=None):
        if not text_data:
            return

        try:
            payload = json.loads(text_data)
        except json.JSONDecodeError:
            await self._send_event('error', {'message': '消息格式错误'})
            return

        event_type = payload.get('type')
        if event_type == 'input':
            await self._handle_input_event(payload)
        elif event_type == 'resize':
            await self._handle_resize_event(payload)
        elif event_type == 'ping':
            await self._send_event('pong', {})
        elif event_type == 'close':
            await self.close()

    async def _handle_input_event(self, payload):
        data = payload.get('data', '')
        if not data or self.ssh_channel is None:
            return

        if not self._is_ssh_alive():
            await self._close_due_to_dead_ssh('SSH 会话已断开')
            return

        await asyncio.to_thread(self.ssh_channel.send, data)
        self.audit_input_bytes += len(data.encode('utf-8', errors='ignore'))
        self.audit_command_count += data.count('\n') + data.count('\r')
        self.audit_stats_dirty = True
        await self._append_audit_content('input', data)

    async def _handle_resize_event(self, payload):
        if self.ssh_channel is None:
            return

        if not self._is_ssh_alive():
            await self._close_due_to_dead_ssh('SSH 会话已断开')
            return

        cols = int(payload.get('cols') or 120)
        rows = int(payload.get('rows') or 32)
        await asyncio.to_thread(self.ssh_channel.resize_pty, width=cols, height=rows)

    async def disconnect(self, close_code):
        self.connected = False
        WebSSHRuntimeRegistry.mark_inactive(self.audit_session_id)
        await self._teardown_ssh_resources()

        await self._flush_content_buffer(force=True)

        if not self.audit_closed:
            await self._close_session_log(close_code=close_code, status=WebSSHSessionLog.Status.CLOSED)

    async def _teardown_ssh_resources(self):
        if self.reader_task is not None:
            self.reader_task.cancel()
            try:
                await self.reader_task
            except asyncio.CancelledError:
                pass
            self.reader_task = None

        if self.ssh_channel is not None:
            await asyncio.to_thread(self.ssh_channel.close)
            self.ssh_channel = None

        if self.ssh_client is not None:
            await asyncio.to_thread(self.ssh_client.close)
            self.ssh_client = None

    async def _reader_loop(self):
        while self.connected and self.ssh_channel is not None:
            if not self._is_ssh_alive():
                break

            try:
                data = await asyncio.to_thread(self.ssh_channel.recv, 4096)
            except socket.timeout:
                if not self._is_ssh_alive():
                    break
                await asyncio.sleep(0.05)
                continue
            except Exception:
                break

            if not data:
                break

            output_text = data.decode('utf-8', errors='ignore')
            await self._send_event('output', {'data': output_text})
            await self._append_audit_content('output', output_text)

        if self.connected:
            await self._close_due_to_dead_ssh('SSH 会话已断开')

    async def _open_ssh(self, host, credential):
        if paramiko is None:
            return False, 'paramiko 未安装，无法建立 SSH 连接'

        pm = paramiko

        def _connect():
            client = pm.SSHClient()
            client.set_missing_host_key_policy(pm.AutoAddPolicy())

            connect_kwargs = {
                'hostname': host.ip,
                'port': host.port or credential.port or 22,
                'username': credential.username,
                'timeout': 15,
                'banner_timeout': 15,
                'allow_agent': False,
                'look_for_keys': False,
            }

            if credential.auth_type == credential.AuthType.SSH_KEY:
                key_data = credential.private_key or ''
                if not key_data:
                    raise ValueError('SSH Key 凭证缺少私钥')
                connect_kwargs['pkey'] = pm.RSAKey.from_private_key(io.StringIO(key_data))
            elif credential.auth_type == credential.AuthType.PASSWORD:
                connect_kwargs['password'] = credential.password
            else:
                raise ValueError('不支持的凭证类型')

            client.connect(**connect_kwargs)
            channel = client.invoke_shell(term='xterm')
            channel.settimeout(0.5)
            return client, channel

        try:
            self.ssh_client, self.ssh_channel = await asyncio.to_thread(_connect)
            return True, ''
        except Exception as exc:
            return False, f'SSH 连接失败：{exc}'

    @database_sync_to_async
    def _get_host_and_default_credential(self, host_id):
        host = Host.objects.select_related('system').filter(id=host_id).first()
        if not host:
            return None, None, '', '主机不存在'

        system = getattr(host, 'system', None)
        hostname = getattr(system, 'hostname', None) if system else None
        host_display_name = host.instance_name or hostname or f'Host-{host_id}'

        relation = HostCredential.objects.filter(host=host, is_default=True).select_related('credential').first()
        if not relation or not relation.credential:
            return None, None, host_display_name, '主机未配置默认 SSH 凭证'

        credential = relation.credential
        if not credential.username:
            return None, None, host_display_name, 'SSH 凭证缺少用户名'

        if credential.auth_type == credential.AuthType.PASSWORD and not credential.password:
            return None, None, host_display_name, 'SSH 凭证缺少密码'

        if credential.auth_type == credential.AuthType.SSH_KEY and not credential.private_key:
            return None, None, host_display_name, 'SSH 凭证缺少私钥'

        return host, credential, host_display_name, ''

    def _get_token_from_query_string(self):
        raw_query = self.scope.get('query_string', b'').decode('utf-8')
        query = parse_qs(raw_query)
        token = query.get('token', [''])[0]
        return token.strip()

    def _decode_token(self, token):
        try:
            jwt_decode_handler = cast(Callable[[str], dict[str, Any]], api_settings.JWT_DECODE_HANDLER)
            return jwt_decode_handler(token)
        except Exception:
            return None

    def _has_webssh_permission(self, payload):
        if payload.get('username') == 'admin':
            return True
        perms = payload.get('perms') or []
        return 'assets:hosts:webssh' in perms or 'assets:hosts:view' in perms

    def _is_ssh_alive(self):
        channel = self.ssh_channel
        client = self.ssh_client
        if channel is None or client is None:
            return False

        try:
            transport = client.get_transport()
            if transport is None or not transport.is_active():
                return False
            if channel.closed:
                return False
            if channel.exit_status_ready():
                return False
            return True
        except Exception:
            return False

    async def _close_due_to_dead_ssh(self, message):
        if not self.audit_close_notified:
            await self._send_event('closed', {'message': message})
            self.audit_close_notified = True
        await self.close()

    async def _send_event(self, event_type, data):
        await self.send(text_data=json.dumps({'type': event_type, **data}))

    async def _append_audit_content(self, direction, text):
        if not text or self.audit_session_pk is None:
            return

        async with self.audit_flush_lock:
            remaining_bytes = max(self.audit_max_content_bytes - self.audit_recorded_content_bytes, 0)
            if remaining_bytes <= 0:
                self.audit_is_content_truncated = True
                self.audit_stats_dirty = True
                await self._sync_live_audit_if_needed_locked(force=False)
                return

            clipped_text, clipped_bytes = self._clip_text_by_utf8_bytes(text, remaining_bytes)
            if not clipped_text:
                self.audit_is_content_truncated = True
                self.audit_stats_dirty = True
                await self._sync_live_audit_if_needed_locked(force=False)
                return

            if direction == 'input':
                self.audit_input_content_chunks.append(clipped_text)
            else:
                self.audit_output_content_chunks.append(clipped_text)

            self.audit_recorded_content_bytes += clipped_bytes
            self.audit_buffered_bytes += clipped_bytes

            if clipped_bytes < len(text.encode('utf-8', errors='ignore')):
                self.audit_is_content_truncated = True
                self.audit_stats_dirty = True

            await self._sync_live_audit_if_needed_locked(force=False)

    async def _flush_content_buffer(self, force=False):
        async with self.audit_flush_lock:
            if force:
                await self._flush_content_buffer_locked()
                return
            if self.audit_buffered_bytes >= self.audit_flush_threshold_bytes:
                await self._flush_content_buffer_locked()

    async def _flush_content_buffer_locked(self):
        await self._sync_live_audit_if_needed_locked(force=True)

    async def _sync_live_audit_if_needed_locked(self, force=False):
        if self.audit_session_pk is None:
            return

        has_content = bool(self.audit_input_content_chunks or self.audit_output_content_chunks)
        should_sync = force or self.audit_buffered_bytes >= self.audit_flush_threshold_bytes
        if not should_sync:
            should_sync = (time.monotonic() - self.audit_last_sync_monotonic) >= self.audit_live_sync_interval_seconds

        if not should_sync and not force:
            return

        input_chunk = ''.join(self.audit_input_content_chunks)
        output_chunk = ''.join(self.audit_output_content_chunks)
        truncated = self.audit_is_content_truncated

        self.audit_input_content_chunks = []
        self.audit_output_content_chunks = []
        self.audit_buffered_bytes = 0

        if has_content:
            await self._append_session_content(input_chunk, output_chunk, self.audit_recorded_content_bytes, truncated)

        if self.audit_stats_dirty or force:
            await self._update_live_session_stats()
            self.audit_stats_dirty = False

        self.audit_last_sync_monotonic = time.monotonic()

    def _get_client_ip(self):
        client = self.scope.get('client')
        if isinstance(client, (tuple, list)) and client:
            return str(client[0])
        return ''

    def _get_header(self, header_name):
        target = header_name.lower().encode('utf-8')
        for key, value in self.scope.get('headers', []):
            if key.lower() == target:
                return value.decode('utf-8', errors='ignore')
        return ''

    @staticmethod
    def _clip_text_by_utf8_bytes(text, max_bytes):
        raw_bytes = text.encode('utf-8', errors='ignore')
        if len(raw_bytes) <= max_bytes:
            return text, len(raw_bytes)

        clipped_bytes = raw_bytes[:max_bytes]
        clipped_text = clipped_bytes.decode('utf-8', errors='ignore')
        clipped_text_bytes = clipped_text.encode('utf-8', errors='ignore')
        return clipped_text, len(clipped_text_bytes)

    @database_sync_to_async
    def _create_session_log(self):
        log = WebSSHSessionLog.objects.create(
            session_id=self.audit_session_id,
            host_id=self.host_id,
            user_id=self.audit_user_id,
            username=self.audit_username,
            client_ip=self.audit_client_ip,
            user_agent=self.audit_user_agent,
            status=WebSSHSessionLog.Status.CONNECTED,
            recorded_content_bytes=0,
            is_content_truncated=False,
        )
        return log.pk, log.start_time

    @database_sync_to_async
    def _append_session_content(self, input_chunk, output_chunk, recorded_bytes, is_truncated):
        updates = {
            'recorded_content_bytes': recorded_bytes,
            'is_content_truncated': is_truncated,
        }

        if input_chunk:
            updates['input_content'] = updates.get('input_content', None)
        if output_chunk:
            updates['output_content'] = updates.get('output_content', None)

        log = WebSSHSessionLog.objects.filter(pk=self.audit_session_pk).first()
        if not log:
            return

        if input_chunk:
            log.input_content = (log.input_content or '') + input_chunk
        if output_chunk:
            log.output_content = (log.output_content or '') + output_chunk

        log.recorded_content_bytes = recorded_bytes
        log.is_content_truncated = is_truncated
        log.save(update_fields=['input_content', 'output_content', 'recorded_content_bytes', 'is_content_truncated'])

    @database_sync_to_async
    def _update_live_session_stats(self):
        if not self.audit_session_pk:
            return

        duration_seconds = None
        if self.audit_started_at:
            duration_seconds = int(max((timezone.now() - self.audit_started_at).total_seconds(), 0))

        WebSSHSessionLog.objects.filter(pk=self.audit_session_pk).update(
            input_bytes=self.audit_input_bytes,
            command_count=self.audit_command_count,
            duration_seconds=duration_seconds,
            recorded_content_bytes=self.audit_recorded_content_bytes,
            is_content_truncated=self.audit_is_content_truncated,
        )

    @database_sync_to_async
    def _load_webssh_audit_configs(self):
        retention_cfg, _ = SysConfig.objects.get_or_create(
            key='sys.audit.webssh.retention_days',
            defaults={
                'value': '30',
                'default_value': '30',
                'value_type': 'int',
                'name': 'WebSSH 审计保留天数',
                'description': 'WebSSH 会话完整记录在数据库中的保留天数',
                'is_readonly': False,
            },
        )
        max_mb_cfg, _ = SysConfig.objects.get_or_create(
            key='sys.audit.webssh.max_content_mb',
            defaults={
                'value': '20',
                'default_value': '20',
                'value_type': 'int',
                'name': 'WebSSH 单会话最大记录大小(MB)',
                'description': '单个 WebSSH 会话完整输入输出记录上限，超出后截断',
                'is_readonly': False,
            },
        )

        try:
            retention_days = max(1, int(str(retention_cfg.value).strip()))
        except (ValueError, TypeError):
            retention_days = 30

        try:
            max_mb = max(1, int(str(max_mb_cfg.value).strip()))
        except (ValueError, TypeError):
            max_mb = 20

        return retention_days, max_mb * 1024 * 1024

    @database_sync_to_async
    def _cleanup_expired_session_logs(self, retention_days):
        cutoff = timezone.now() - timedelta(days=retention_days)
        WebSSHSessionLog.objects.filter(start_time__lt=cutoff).delete()

    @database_sync_to_async
    def _close_session_log(self, close_code=None, status=WebSSHSessionLog.Status.CLOSED, error_message=''):
        if self.audit_closed or not self.audit_session_pk:
            return

        end_time = timezone.now()
        duration_seconds = None
        if self.audit_started_at:
            duration_seconds = int(max((end_time - self.audit_started_at).total_seconds(), 0))

        WebSSHSessionLog.objects.filter(pk=self.audit_session_pk).update(
            end_time=end_time,
            duration_seconds=duration_seconds,
            close_code=close_code,
            status=status,
            error_message=error_message or '',
            input_bytes=self.audit_input_bytes,
            command_count=self.audit_command_count,
            recorded_content_bytes=self.audit_recorded_content_bytes,
            is_content_truncated=self.audit_is_content_truncated,
        )
        self.audit_closed = True
