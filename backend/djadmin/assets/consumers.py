import asyncio
import importlib
import json
import time
import warnings
from datetime import timedelta
from typing import Any, Callable, cast
from urllib.parse import parse_qs

from channels.db import database_sync_to_async
from channels.generic.websocket import AsyncWebsocketConsumer
from cryptography.utils import CryptographyDeprecationWarning
from django.conf import settings
from django.utils import timezone
from rest_framework_jwt.settings import api_settings

from .models import Host, HostCredential, WebSSHSessionLog
from .credential_crypto import decrypt_secret
from .webssh_runtime import WebSSHRuntimeRegistry
from sys_config.models import SysConfig

warnings.filterwarnings(
    'ignore',
    message='.*TripleDES has been moved to cryptography\\.hazmat\\.decrepit\\.ciphers\\.algorithms\\.TripleDES.*',
    category=CryptographyDeprecationWarning,
)

try:
    from nats.aio.client import Client as NATS
except ImportError:  # pragma: no cover
    NATS = None

try:
    asyncssh = importlib.import_module('asyncssh')
except ImportError:  # pragma: no cover
    asyncssh = None


class HostWebSSHConsumer(AsyncWebsocketConsumer):
    """WebSSH over agent bridge (frontend WS <-> backend WS consumer <-> NATS <-> agent PTY)."""

    async def connect(self):
        route = self.scope.get('url_route')
        kwargs = route.get('kwargs') if isinstance(route, dict) else None
        host_id_value = kwargs.get('host_id') if isinstance(kwargs, dict) else None
        if host_id_value is None:
            await self.close(code=4400)
            return

        self.host_id = int(host_id_value)
        self.connected = False
        self.nats_client = None
        self.nats_subscription = None
        self.ssh_conn = None
        self.ssh_process = None
        self.ssh_output_task = None
        self.agent_id = ''
        self.selected_credential_id = None
        self.session_id = f'term-{self.host_id}-{int(time.time() * 1000)}'
        self.term_ready_event = asyncio.Event()
        self.term_connected_payload = {}
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

        self.token_expire_at = self._extract_token_expire_at(payload)
        if self._is_token_expired():
            await self.close(code=4401)
            return

        self.audit_user_id = payload.get('user_id')
        self.audit_username = payload.get('username') or ''
        self.audit_client_ip = self._get_client_ip()
        self.audit_user_agent = self._get_header('user-agent')
        self.selected_credential_id = self._get_credential_id_from_query_string()

        host, host_display_name, agent_id, error_msg = await self._get_host_and_agent(self.host_id)
        if error_msg:
            await self.accept()
            await self._send_event('error', {'message': error_msg})
            await self.close(code=4404)
            return
        if host is None:
            await self.close(code=4404)
            return

        self.agent_id = agent_id

        await self.accept()

        self.audit_retention_days, self.audit_max_content_bytes, idle_timeout_seconds = await self._load_webssh_audit_configs()
        self.heartbeat_timeout_seconds = idle_timeout_seconds
        await self._cleanup_expired_session_logs(self.audit_retention_days)
        self.audit_session_pk, self.audit_started_at = await self._create_session_log()

        ok, error = await self._open_ssh_terminal(host, host_display_name, host.ip)
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
        WebSSHRuntimeRegistry.mark_active(self.audit_session_pk, self, self.host_id)

        self.heartbeat_task = asyncio.create_task(self._heartbeat_watchdog())
        if self.token_expire_at is not None:
            self.token_expiry_task = asyncio.create_task(self._token_expiry_watchdog())

    async def receive(self, text_data=None, bytes_data=None):
        if not text_data:
            return

        if self._is_token_expired():
            if not self.audit_close_notified:
                await self._send_event('closed', {'message': '登录已过期，请重新登录后再连接'})
                self.audit_close_notified = True
            await self.close(code=4401)
            return

        try:
            payload = json.loads(text_data)
        except json.JSONDecodeError:
            await self._send_event('error', {'message': '消息格式错误'})
            return

        event_type = payload.get('type')
        if event_type == 'input':
            self.last_client_activity_monotonic = time.monotonic()
            data = payload.get('data', '')
            if data:
                self.audit_input_bytes += len(str(data).encode('utf-8', errors='ignore'))
                self.audit_command_count += str(data).count('\n') + str(data).count('\r')
                await self._append_audit_content('input', str(data))
                await self._write_ssh_input(str(data))
        elif event_type == 'resize':
            self.last_client_activity_monotonic = time.monotonic()
            await self._resize_ssh_terminal(
                int(payload.get('cols') or 120),
                int(payload.get('rows') or 32),
            )
        elif event_type == 'transfer_activity':
            self.last_client_activity_monotonic = time.monotonic()
        elif event_type == 'ping':
            await self._send_event('pong', {})
        elif event_type == 'close':
            await self.close()

    async def disconnect(self, close_code):
        self.connected = False
        WebSSHRuntimeRegistry.mark_inactive(self.audit_session_pk)

        if self.token_expiry_task is not None:
            self.token_expiry_task.cancel()
            try:
                await self.token_expiry_task
            except asyncio.CancelledError:
                pass
            self.token_expiry_task = None

        if self.heartbeat_task is not None:
            self.heartbeat_task.cancel()
            try:
                await self.heartbeat_task
            except asyncio.CancelledError:
                pass
            self.heartbeat_task = None

        await self._close_ssh_session()

        await self._close_session_log(close_code=close_code, status=WebSSHSessionLog.Status.CLOSED)

    async def _open_ssh_terminal(self, host, host_display_name, host_ip):
        if asyncssh is None:
            return False, 'asyncssh 未安装，无法建立 SSH 终端'

        credential, error_msg = await self._resolve_credential_for_host(host.id, self.selected_credential_id)
        if error_msg:
            return False, error_msg
        if not credential:
            return False, '主机未配置可用 SSH 凭证'

        connect_kwargs = {
            'host': host_ip,
            'port': int(host.port or credential.get('port') or 22),
            'username': credential.get('username'),
            'known_hosts': None,
        }
        auth_type = int(credential.get('auth_type') or 0)
        if auth_type == 2:
            connect_kwargs['client_keys'] = [str(credential.get('private_key') or '').encode('utf-8')]
        else:
            connect_kwargs['password'] = credential.get('password')

        try:
            self.ssh_conn = await asyncssh.connect(**connect_kwargs)
            self.ssh_process = await self.ssh_conn.create_process(
                term_type='xterm',
                term_size=(120, 32),
            )
            self.ssh_output_task = asyncio.create_task(self._ssh_output_loop())

            await self._send_event(
                'connected',
                {
                    'host_id': self.host_id,
                    'instance_name': host_display_name,
                    'ip': host_ip,
                    'log_id': self.audit_session_pk,
                    'home_dir': '/root',
                    # Re-enable legacy direct SFTP file operations for non-agent WebSSH mode.
                    'supports_file_ops': True,
                    'terminal_mode': 'ssh_credential',
                    'credential_id': credential.get('id'),
                },
            )
            return True, ''
        except Exception as exc:
            await self._close_ssh_session()
            return False, f'SSH 终端建立失败：{exc}'

    async def _ssh_output_loop(self):
        try:
            while self.connected and self.ssh_process and self.ssh_process.stdout:
                output = await self.ssh_process.stdout.read(4096)
                if not output:
                    break
                data = str(output)
                await self._append_audit_content('output', data)
                await self._send_event('output', {'data': data})
        except Exception as exc:
            await self._send_event('error', {'message': f'SSH 输出读取失败：{exc}'})
        finally:
            if self.connected and not self.audit_close_notified:
                self.audit_close_notified = True
                await self._send_event('closed', {'message': 'SSH 会话已关闭'})
                await self.close(code=4000)

    async def _write_ssh_input(self, data):
        if not self.ssh_process or not getattr(self.ssh_process, 'stdin', None):
            return
        try:
            self.ssh_process.stdin.write(data)
        except Exception as exc:
            await self._send_event('error', {'message': f'SSH 输入失败：{exc}'})

    async def _resize_ssh_terminal(self, cols, rows):
        if not self.ssh_process:
            return
        safe_cols = int(cols or 120)
        safe_rows = int(rows or 32)
        try:
            resize_fn = getattr(self.ssh_process, 'set_terminal_size', None)
            if callable(resize_fn):
                resize_fn(safe_cols, safe_rows)
                return
            channel = getattr(self.ssh_process, 'channel', None)
            if channel and hasattr(channel, 'change_terminal_size'):
                channel.change_terminal_size(safe_cols, safe_rows)
        except Exception:
            # Ignore resize errors to keep session usable.
            return

    async def _close_ssh_session(self):
        if self.ssh_output_task is not None:
            self.ssh_output_task.cancel()
            try:
                await self.ssh_output_task
            except asyncio.CancelledError:
                pass
            self.ssh_output_task = None

        if self.ssh_process is not None:
            try:
                if getattr(self.ssh_process, 'stdin', None):
                    self.ssh_process.stdin.write_eof()
            except Exception:
                pass
            self.ssh_process = None

        if self.ssh_conn is not None:
            try:
                self.ssh_conn.close()
                await self.ssh_conn.wait_closed()
            except Exception:
                pass
            self.ssh_conn = None

    @database_sync_to_async
    def _resolve_credential_for_host(self, host_id, selected_credential_id):
        relation = None
        if selected_credential_id not in (None, '', 0, '0'):
            relation = HostCredential.objects.filter(
                host_id=host_id,
                credential_id=int(selected_credential_id),
            ).select_related('credential').first()

        if relation is None:
            relation = HostCredential.objects.filter(host_id=host_id, is_default=True).select_related('credential').first()

        if relation is None or relation.credential is None:
            return None, '主机未配置默认 SSH 凭证，请先在主机配置中绑定凭证'

        credential = relation.credential
        if not credential.username:
            return None, 'SSH 凭证缺少用户名'
        decrypted_password = ''
        if credential.auth_type == credential.AuthType.PASSWORD:
            try:
                decrypted_password = str(decrypt_secret(credential.password) or '')
            except ValueError as exc:
                return None, str(exc)

        if credential.auth_type == credential.AuthType.PASSWORD and not decrypted_password:
            return None, 'SSH 凭证缺少密码'
        if credential.auth_type == credential.AuthType.SSH_KEY and not credential.private_key:
            return None, 'SSH 凭证缺少私钥'

        return {
            'id': credential.id,
            'username': credential.username,
            'password': decrypted_password,
            'private_key': credential.private_key,
            'port': credential.port,
            'auth_type': credential.auth_type,
        }, ''

    async def _handle_term_event_message(self, raw_data):
        try:
            payload = json.loads(raw_data.decode('utf-8'))
        except Exception:
            return

        event_type = str(payload.get('type') or '').strip().lower()
        if event_type in {'connected', 'ready'}:
            self.term_connected_payload = payload
            self.term_ready_event.set()
            return

        if event_type == 'output':
            output = str(payload.get('data') or '')
            if output:
                await self._append_audit_content('output', output)
            await self._send_event('output', {'data': output})
            return

        if event_type == 'error':
            message = str(payload.get('message') or 'Agent 终端异常')
            await self._send_event('error', {'message': message})
            return

        if event_type == 'closed':
            message = str(payload.get('message') or '会话已关闭')
            if not self.audit_close_notified:
                await self._send_event('closed', {'message': message})
                self.audit_close_notified = True
            await self.close(code=4000)
            return

    async def _heartbeat_watchdog(self):
        while self.connected:
            await asyncio.sleep(self.heartbeat_check_interval_seconds)
            if not self.connected:
                return

            idle_seconds = time.monotonic() - self.last_client_activity_monotonic
            if idle_seconds <= self.heartbeat_timeout_seconds:
                continue

            await self._send_event('closed', {'message': '连接超时，已自动断开'})
            self.audit_close_notified = True
            await self.close(code=4000)
            return

    async def _token_expiry_watchdog(self):
        while self.connected:
            if self.token_expire_at is None:
                return

            remaining_seconds = self.token_expire_at - time.time()
            if remaining_seconds <= 0:
                if not self.audit_close_notified:
                    await self._send_event('closed', {'message': '登录已过期，请重新登录后再连接'})
                    self.audit_close_notified = True
                await self.close(code=4401)
                return

            await asyncio.sleep(min(remaining_seconds, 30))

    def _init_audit_runtime_state(self):
        self.audit_user_id = None
        self.audit_username = ''
        self.audit_client_ip = ''
        self.audit_user_agent = ''
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
        self.audit_close_notified = False
        self.heartbeat_timeout_seconds = 30 * 60
        self.heartbeat_check_interval_seconds = 10
        self.last_client_activity_monotonic = time.monotonic()
        self.heartbeat_task = None
        self.token_expire_at = None
        self.token_expiry_task = None

    @database_sync_to_async
    def _get_host_and_agent(self, host_id):
        host = Host.objects.select_related('system').filter(id=host_id).first()
        if not host:
            return None, '', '', '主机不存在'

        system = getattr(host, 'system', None)
        hostname = getattr(system, 'hostname', None) if system else None
        host_display_name = host.instance_name or hostname or f'Host-{host_id}'

        agent_id = str(host.instance_name or '').strip()
        # WebSSH 入口统一要求 Agent 在线，避免离线主机仍可建立终端会话。
        system_collector_source = str(getattr(system, 'collector_source', '') or '').strip().lower() if system else ''
        collect_time = getattr(host, 'collect_time', None)
        heartbeat_threshold = timezone.now() - timedelta(seconds=30)
        is_agent_online = bool(
            system_collector_source == 'agent'
            and collect_time
            and collect_time >= heartbeat_threshold
        )
        if not is_agent_online:
            return None, host_display_name, agent_id, 'Agent 离线，禁止打开 WebSSH'

        return host, host_display_name, agent_id, ''

    def _get_token_from_query_string(self):
        raw_query = self.scope.get('query_string', b'').decode('utf-8')
        query = parse_qs(raw_query)
        token = query.get('token', [''])[0]
        return token.strip()

    def _get_credential_id_from_query_string(self):
        raw_query = self.scope.get('query_string', b'').decode('utf-8')
        query = parse_qs(raw_query)
        credential_raw = (query.get('credential_id', [''])[0] or '').strip()
        if credential_raw == '':
            return None
        try:
            parsed = int(credential_raw)
        except (TypeError, ValueError):
            return None
        return parsed if parsed > 0 else None

    def _decode_token(self, token):
        try:
            jwt_decode_handler = cast(Callable[[str], dict[str, Any]], api_settings.JWT_DECODE_HANDLER)
            return jwt_decode_handler(token)
        except Exception:
            return None

    def _extract_token_expire_at(self, payload):
        exp = payload.get('exp') if isinstance(payload, dict) else None
        if exp in [None, '']:
            return None
        try:
            return float(str(exp))
        except (TypeError, ValueError):
            return None

    def _is_token_expired(self):
        if self.token_expire_at is None:
            return False
        return time.time() >= self.token_expire_at

    def _has_webssh_permission(self, payload):
        if payload.get('username') == 'admin':
            return True
        perms = payload.get('perms') or []
        return 'assets:hosts:webssh' in perms or 'assets:hosts:view' in perms

    async def _send_event(self, event_type, data):
        await self.send(text_data=json.dumps({'type': event_type, **data}))

    async def _append_audit_content(self, direction, text):
        if not text or self.audit_session_pk is None:
            return

        chunk = str(text)
        chunk_bytes = len(chunk.encode('utf-8', errors='ignore'))
        remaining_bytes = max(self.audit_max_content_bytes - self.audit_recorded_content_bytes, 0)
        if remaining_bytes <= 0:
            self.audit_is_content_truncated = True
            return

        if chunk_bytes > remaining_bytes:
            raw_bytes = chunk.encode('utf-8', errors='ignore')[:remaining_bytes]
            chunk = raw_bytes.decode('utf-8', errors='ignore')
            chunk_bytes = len(chunk.encode('utf-8', errors='ignore'))
            self.audit_is_content_truncated = True

        self.audit_recorded_content_bytes += chunk_bytes

        if direction == 'input':
            self.audit_input_content_chunks.append(chunk)
        else:
            self.audit_output_content_chunks.append(chunk)

        await self._append_session_content(chunk if direction == 'input' else '', chunk if direction == 'output' else '')

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

    @database_sync_to_async
    def _create_session_log(self):
        log = WebSSHSessionLog.objects.create(
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
    def _append_session_content(self, input_chunk, output_chunk):
        log = WebSSHSessionLog.objects.filter(pk=self.audit_session_pk).first()
        if not log:
            return

        if input_chunk:
            log.input_content = (log.input_content or '') + input_chunk
        if output_chunk:
            log.output_content = (log.output_content or '') + output_chunk

        log.recorded_content_bytes = self.audit_recorded_content_bytes
        log.is_content_truncated = self.audit_is_content_truncated
        log.input_bytes = self.audit_input_bytes
        log.command_count = self.audit_command_count
        log.save(
            update_fields=[
                'input_content',
                'output_content',
                'recorded_content_bytes',
                'is_content_truncated',
                'input_bytes',
                'command_count',
            ]
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
        idle_timeout_cfg, _ = SysConfig.objects.get_or_create(
            key='sys.webssh.idle_timeout_minutes',
            defaults={
                'value': '30',
                'default_value': '30',
                'value_type': 'int',
                'name': 'WebSSH 空闲断开时长(分钟)',
                'description': '客户端无终端输入/窗口调整达到该时长后自动断开',
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

        try:
            idle_timeout_minutes = max(1, int(str(idle_timeout_cfg.value).strip()))
        except (ValueError, TypeError):
            idle_timeout_minutes = 30

        return retention_days, max_mb * 1024 * 1024, idle_timeout_minutes * 60

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
