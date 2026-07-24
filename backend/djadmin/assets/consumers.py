import asyncio
import codecs
import json
import logging
import re
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

from .models import Host, WebSSHSessionLog
from .webssh_runtime import WebSSHRuntimeRegistry
from .grpc_transfer.client import AgentChannelClient, AgentGrpcTransferError
from sys_config.models import SysConfig

warnings.filterwarnings(
    'ignore',
    message='.*TripleDES has been moved to cryptography\\.hazmat\\.decrepit\\.ciphers\\.algorithms\\.TripleDES.*',
    category=CryptographyDeprecationWarning,
)

logger = logging.getLogger(__name__)


class HostWebSSHConsumer(AsyncWebsocketConsumer):
    """WebSSH 终端：前端 WS <-> 后端 consumer <-> agent gRPC 长连接 <-> 目标主机本地 PTY。

    终端 I/O 全程走 agent（agent 主动拨入的 AgentChannel.Session），backend 不再直连
    目标主机 SSH。因此终端以 agent 进程用户（通常 root）打开本地 shell，不需要 SSH 凭证。
    准入要求主机 agent 在线且已建立 gRPC 通道，否则拒绝打开。
    """

    async def connect(self):
        route = self.scope.get('url_route')
        kwargs = route.get('kwargs') if isinstance(route, dict) else None
        host_id_value = kwargs.get('host_id') if isinstance(kwargs, dict) else None
        if host_id_value is None:
            await self.close(code=4400)
            return

        self.host_id = int(host_id_value)
        self.connected = False
        self.agent_client = None
        self.term_session = None
        self.term_output_task = None
        # agent 回传的 PTY 输出是按字节分片的，多字节 UTF-8 可能跨分片，用增量解码器
        # 避免在分片边界把一个字符拆坏（例如中文/emoji 显示为乱码）。
        self._term_decoder = codecs.getincrementaldecoder('utf-8')('replace')
        self.session_id = f'term-{self.host_id}-{int(time.time() * 1000)}'
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
        query = self._parse_query_string()
        raw_query = str(self.scope.get('query_string', b'').decode('utf-8', errors='ignore') or '')
        raw_target_user = str(query.get('target_user', [''])[0] or '').strip()
        logger.info(
            '[webssh] connect query parsed: host_id=%s raw_query=%s target_user=%s',
            self.host_id,
            raw_query,
            raw_target_user,
        )
        self.audit_requested_username = self._get_target_user_from_query_string()
        if raw_target_user and not self.audit_requested_username:
            await self.accept()
            await self._send_event('error', {'message': 'target_user 参数格式非法'})
            await self.close(code=4400)
            return
        self.audit_effective_username = ''
        self.audit_switch_user_status = 'none'
        self.audit_switch_user_error = ''
        self.audit_client_ip = self._get_client_ip()
        self.audit_user_agent = self._get_header('user-agent')

        host, host_display_name, error_msg = await self._get_host_and_agent(self.host_id)
        if error_msg:
            await self.accept()
            await self._send_event('error', {'message': error_msg})
            await self.close(code=4404)
            return
        if host is None:
            await self.close(code=4404)
            return

        await self.accept()

        self.audit_retention_days, self.audit_max_content_bytes, idle_timeout_seconds = await self._load_webssh_audit_configs()
        self.heartbeat_timeout_seconds = idle_timeout_seconds
        await self._cleanup_expired_session_logs(self.audit_retention_days)
        self.audit_session_pk, self.audit_started_at = await self._create_session_log()

        ok, error = await self._open_agent_terminal(host, host_display_name, host.ip)
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
        self.audit_flush_task = asyncio.create_task(self._audit_flush_loop())

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
                await self._write_terminal_input(str(data))
        elif event_type == 'resize':
            self.last_client_activity_monotonic = time.monotonic()
            await self._resize_terminal(
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

        if self.audit_flush_task is not None:
            self.audit_flush_task.cancel()
            try:
                await self.audit_flush_task
            except asyncio.CancelledError:
                pass
            self.audit_flush_task = None

        await self._close_agent_terminal()
        await self._flush_content_buffer(force=True)

        await self._close_session_log(close_code=close_code, status=WebSSHSessionLog.Status.CLOSED)

    async def _open_agent_terminal(self, host, host_display_name, host_ip):
        # 终端全程走 agent gRPC 通道：backend 不再直连目标主机 SSH，
        # 由 agent 在目标主机本地开一个登录交互 shell（以 agent 进程用户身份，通常 root）。
        agent_id = str(getattr(host, 'instance_name', '') or '').strip()
        if not agent_id:
            return False, '主机未绑定 agent 标识（instance_name 为空）'

        try:
            # AgentChannelClient 构造 + open_shell 都会阻塞（open_shell 等待 agent 的 open ack），
            # 放到线程里执行，避免阻塞 consumer 的事件循环。
            self.agent_client, self.term_session = await asyncio.to_thread(
                self._blocking_open_agent_terminal,
                agent_id,
                120,
                32,
                self.audit_requested_username,
            )
        except AgentGrpcTransferError as exc:
            if self.audit_requested_username:
                self.audit_switch_user_status = 'failed'
                self.audit_switch_user_error = str(exc)
            return False, self._build_user_friendly_open_error(str(exc), self.audit_requested_username)
        except Exception as exc:
            if self.audit_requested_username:
                self.audit_switch_user_status = 'failed'
                self.audit_switch_user_error = str(exc)
            return False, self._build_user_friendly_open_error(str(exc), self.audit_requested_username)

        effective_user = str(getattr(self.term_session, 'effective_user', '') or '').strip()
        if self.audit_requested_username:
            # 强校验：请求了 target_user 时，agent 必须明确返回 effective_user 且与请求一致。
            # 否则通常是 agent 版本过低（忽略 target_user）或切换未生效，不能静默退回 root。
            if not effective_user:
                await self._close_agent_terminal()
                self.audit_switch_user_status = 'failed'
                self.audit_switch_user_error = 'agent 未返回 effective_user，可能版本过低或未支持按用户切换'
                logger.warning(
                    '[webssh] user-switch failed: host_id=%s requested=%s effective=(empty) reason=%s',
                    self.host_id,
                    self.audit_requested_username,
                    self.audit_switch_user_error,
                )
                return False, 'Agent 不支持按指定用户打开终端，请升级 dj-agent 后重试'
            if effective_user != self.audit_requested_username:
                await self._close_agent_terminal()
                self.audit_switch_user_status = 'failed'
                self.audit_switch_user_error = (
                    f'请求用户 {self.audit_requested_username}，实际生效用户 {effective_user}'
                )
                logger.warning(
                    '[webssh] user-switch failed: host_id=%s requested=%s effective=%s',
                    self.host_id,
                    self.audit_requested_username,
                    effective_user,
                )
                return False, (
                    f'切换用户失败：请求 {self.audit_requested_username}，实际生效 {effective_user}'
                )
            self.audit_switch_user_status = 'success'
            self.audit_switch_user_error = ''
            logger.info(
                '[webssh] user-switch success: host_id=%s requested=%s effective=%s',
                self.host_id,
                self.audit_requested_username,
                effective_user,
            )

        self.audit_effective_username = effective_user or self.audit_username or ''

        self.connected = True
        self.term_output_task = asyncio.create_task(self._agent_output_loop())

        await self._send_event(
            'connected',
            {
                'host_id': self.host_id,
                'instance_name': host_display_name,
                'ip': host_ip,
                'log_id': self.audit_session_pk,
                'home_dir': '/root',
                # 文件操作走 agent gRPC（webssh_host_mixin），前端保留文件面板能力。
                'supports_file_ops': True,
                'terminal_mode': 'agent',
                'requested_user': self.audit_requested_username,
                'effective_user': self.audit_effective_username,
                'switch_user_status': self.audit_switch_user_status,
            },
        )
        return True, ''

    @staticmethod
    def _build_user_friendly_open_error(raw_error, requested_user=''):
        text = str(raw_error or '').strip()
        target = str(requested_user or '').strip()
        low = text.lower()

        if 'unknown user' in low or 'does not exist' in low or 'no such user' in low:
            if target:
                return f'用户 {target} 不存在，无法登录该主机'
            return '目标用户不存在，无法登录该主机'

        if '目标用户不在允许列表中' in text:
            if target:
                return f'用户 {target} 不在允许登录名单中，请联系管理员放行'
            return '目标用户不在允许登录名单中，请联系管理员放行'

        if 'agent 非 root 运行，无法切换到其他系统用户' in text:
            if target:
                return f'当前 Agent 非 root 运行，无法切换到用户 {target}'
            return '当前 Agent 非 root 运行，无法切换到指定用户'

        if 'agent 未返回 effective_user' in text or '不支持按指定用户打开终端' in text:
            return '当前 Agent 版本不支持按指定用户登录，请升级 Agent 后重试'

        if target:
            return f'无法以用户 {target} 打开终端：{text or "未知错误"}'
        return f'终端建立失败：{text or "未知错误"}'

    @staticmethod
    def _blocking_open_agent_terminal(agent_id, cols, rows, target_user=''):
        client = AgentChannelClient(agent_id)
        term_session = client.open_shell(cols=cols, rows=rows, target_user=target_user)
        return client, term_session

    async def _agent_output_loop(self):
        try:
            while self.connected and self.term_session is not None:
                # 在线程里阻塞等待下一帧（带 1s 超时，便于周期性检查 connected 与退出条件）。
                frame = await asyncio.to_thread(self.term_session.recv, 1.0)
                if isinstance(frame, str):
                    # recv 在超时（queue.Empty）时返回 'timeout' 字符串哨兵。
                    continue
                if frame is None:
                    # agent 通道断开：registry 关闭会话时会向等待队列投递 None。
                    break
                kind = frame.WhichOneof('payload')
                if kind == 'terminal_data_response':
                    # 多字节 UTF-8 可能跨帧被切分，使用增量解码器避免乱码。
                    text = self._term_decoder.decode(frame.terminal_data_response.data)
                    if text:
                        await self._append_audit_content('output', text)
                        await self._send_event('output', {'data': text})
                elif kind == 'terminal_exit_response':
                    break
        except Exception as exc:
            await self._send_event('error', {'message': f'终端输出读取失败：{exc}'})
        finally:
            if self.connected and not self.audit_close_notified:
                self.audit_close_notified = True
                await self._send_event('closed', {'message': '终端会话已关闭'})
                await self.close(code=4000)

    async def _write_terminal_input(self, data):
        if self.term_session is None:
            return
        try:
            # send_stdin 仅向发送队列投递帧（非阻塞），可直接在事件循环中调用。
            self.term_session.send_stdin(data)
        except Exception as exc:
            await self._send_event('error', {'message': f'终端输入失败：{exc}'})

    async def _resize_terminal(self, cols, rows):
        if self.term_session is None:
            return
        try:
            self.term_session.resize(int(cols or 120), int(rows or 32))
        except Exception:
            # 忽略 resize 异常，保持会话可用。
            return

    async def _close_agent_terminal(self):
        if self.term_output_task is not None:
            self.term_output_task.cancel()
            try:
                await self.term_output_task
            except asyncio.CancelledError:
                pass
            self.term_output_task = None

        if self.term_session is not None:
            try:
                self.term_session.close()
            except Exception:
                pass
            self.term_session = None
        self.agent_client = None

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
        self.audit_requested_username = ''
        self.audit_effective_username = ''
        self.audit_switch_user_status = 'none'
        self.audit_switch_user_error = ''
        self.audit_client_ip = ''
        self.audit_user_agent = ''
        self.audit_session_pk = None
        self.audit_started_at = None
        self.audit_input_bytes = 0
        self.audit_command_count = 0
        self.audit_closed = False
        self.audit_input_content_chunks = []
        self.audit_output_content_chunks = []
        self.audit_flush_task = None
        self.audit_flush_interval_seconds = 0.5
        self.audit_flush_min_bytes = 8192
        self.audit_last_flush_monotonic = time.monotonic()
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
            return None, '', '主机不存在'

        system = getattr(host, 'system', None)
        hostname = getattr(system, 'hostname', None) if system else None
        host_display_name = host.instance_name or hostname or f'Host-{host_id}'

        # WebSSH 准入仅以 Host.agent_online 字段为准。
        is_agent_online = bool(getattr(host, 'agent_online', False))
        if not is_agent_online:
            return None, host_display_name, 'Agent 离线，禁止打开 WebSSH'

        return host, host_display_name, ''

    def _parse_query_string(self):
        raw_query = self.scope.get('query_string', b'').decode('utf-8')
        return parse_qs(raw_query)

    def _get_token_from_query_string(self):
        query = self._parse_query_string()
        token = query.get('token', [''])[0]
        return token.strip()

    def _get_target_user_from_query_string(self):
        query = self._parse_query_string()
        target_user = str(query.get('target_user', [''])[0] or '').strip()
        if not target_user:
            return ''
        if not re.match(r'^[a-zA-Z_][a-zA-Z0-9._-]{0,63}$', target_user):
            return ''
        return target_user

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

        await self._flush_content_buffer(force=False)

    async def _audit_flush_loop(self):
        while self.connected:
            await asyncio.sleep(self.audit_flush_interval_seconds)
            if not self.connected:
                return
            await self._flush_content_buffer(force=False)

    async def _flush_content_buffer(self, force=False):
        if self.audit_session_pk is None:
            return

        input_chunk = ''.join(self.audit_input_content_chunks)
        output_chunk = ''.join(self.audit_output_content_chunks)
        pending_bytes = len(input_chunk.encode('utf-8', errors='ignore')) + len(output_chunk.encode('utf-8', errors='ignore'))

        if not input_chunk and not output_chunk:
            return

        if not force and pending_bytes < self.audit_flush_min_bytes:
            return

        self.audit_input_content_chunks.clear()
        self.audit_output_content_chunks.clear()
        self.audit_last_flush_monotonic = time.monotonic()
        await self._append_session_content(input_chunk, output_chunk)

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
            requested_username=self.audit_requested_username,
            effective_username=self.audit_effective_username,
            switch_user_status=self.audit_switch_user_status,
            switch_user_error=self.audit_switch_user_error,
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
            requested_username=self.audit_requested_username,
            effective_username=self.audit_effective_username,
            switch_user_status=self.audit_switch_user_status,
            switch_user_error=self.audit_switch_user_error,
            input_bytes=self.audit_input_bytes,
            command_count=self.audit_command_count,
            recorded_content_bytes=self.audit_recorded_content_bytes,
            is_content_truncated=self.audit_is_content_truncated,
        )
        self.audit_closed = True
