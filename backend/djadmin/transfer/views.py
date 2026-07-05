import io
import json
import logging
import posixpath
import re
import shlex
import stat
import time
import uuid
from hashlib import sha256
from urllib.parse import quote

import jwt
from cryptography.utils import CryptographyDeprecationWarning
from django.conf import settings
from django.http import HttpResponse, JsonResponse, StreamingHttpResponse
from django.views import View

from assets.models import Host, HostCredential
from .connection_pool import SSHConnectionPool
from .tokens import parse_download_ticket, parse_upload_ticket

import warnings

warnings.filterwarnings(
    'ignore',
    message='.*TripleDES has been moved to cryptography\\.hazmat\\.decrepit\\.ciphers\\.algorithms\\.TripleDES.*',
    category=CryptographyDeprecationWarning,
)

try:
    import paramiko
except ImportError:  # pragma: no cover
    paramiko = None

SSH_POOL = SSHConnectionPool(
    max_per_key=int(getattr(settings, 'TRANSFER_SSH_POOL_MAX_PER_KEY', 2)),
    idle_seconds=int(getattr(settings, 'TRANSFER_SSH_POOL_IDLE_SECONDS', 120)),
)
logger = logging.getLogger(__name__)

TRANSFER_STREAM_FIRST_CHUNK_BYTES = max(int(getattr(settings, 'TRANSFER_STREAM_FIRST_CHUNK_BYTES', 256 * 1024)), 64 * 1024)
TRANSFER_STREAM_CHUNK_BYTES = max(int(getattr(settings, 'TRANSFER_STREAM_CHUNK_BYTES', 8 * 1024 * 1024)), 512 * 1024)
TRANSFER_STREAM_PROGRESS_LOG_SECONDS = max(int(getattr(settings, 'TRANSFER_STREAM_PROGRESS_LOG_SECONDS', 5)), 1)
TRANSFER_SFTP_WINDOW_SIZE = max(int(getattr(settings, 'TRANSFER_SFTP_WINDOW_SIZE', 8 * 1024 * 1024)), 1024 * 1024)
TRANSFER_SFTP_MAX_PACKET_SIZE = max(int(getattr(settings, 'TRANSFER_SFTP_MAX_PACKET_SIZE', 256 * 1024)), 32 * 1024)
TRANSFER_SFTP_PREFETCH_REQUESTS = max(int(getattr(settings, 'TRANSFER_SFTP_PREFETCH_REQUESTS', 32)), 1)


def _safe_upload_id(upload_id):
    value = str(upload_id or '').strip()
    if not value:
        return ''
    return value if re.fullmatch(r'[A-Za-z0-9._-]{1,128}', value) else ''


class TransferBaseView(View):
    @staticmethod
    def _ok(data=None):
        return JsonResponse({'code': 200, 'msg': 'success', 'data': data or {}})

    @staticmethod
    def _error(message, code=400):
        return JsonResponse({'code': code, 'msg': str(message), 'data': {}})

    @staticmethod
    def _stream_error(message, status=400):
        return HttpResponse(message, status=status, content_type='text/plain; charset=utf-8')

    @staticmethod
    def _release_sftp(ssh_client, sftp_client, pool_key=None, broken=False):
        if sftp_client is not None:
            try:
                sftp_client.close()
            except Exception:
                pass
        if ssh_client is not None:
            if broken or not pool_key:
                SSH_POOL.discard(ssh_client)
            else:
                SSH_POOL.release(pool_key, ssh_client)

    @staticmethod
    def _get_default_credential(host):
        relation = HostCredential.objects.filter(host=host, is_default=True).select_related('credential').first()
        if not relation or not relation.credential:
            raise ValueError('主机未配置默认 SSH 凭证')
        credential = relation.credential
        if not credential.username:
            raise ValueError('SSH 凭证缺少用户名')
        if credential.auth_type == credential.AuthType.PASSWORD and not credential.password:
            raise ValueError('SSH 凭证缺少密码')
        if credential.auth_type == credential.AuthType.SSH_KEY and not credential.private_key:
            raise ValueError('SSH 凭证缺少私钥')
        return credential

    @staticmethod
    def _connect_sftp(host):
        if paramiko is None:
            raise RuntimeError('paramiko 未安装，无法传输文件')
        credential = TransferBaseView._get_default_credential(host)
        port = host.port or credential.port or 22
        auth_type = credential.auth_type
        if auth_type == credential.AuthType.SSH_KEY:
            auth_secret = credential.private_key or ''
        else:
            auth_secret = credential.password or ''
        auth_fingerprint = sha256(str(auth_secret).encode('utf-8')).hexdigest()
        pool_key = f'{host.id}:{host.ip}:{port}:{credential.username}:{auth_type}:{auth_fingerprint}'
        connect_kwargs = {
            'hostname': host.ip,
            'port': port,
            'username': credential.username,
            'timeout': 15,
            'banner_timeout': 15,
            'allow_agent': False,
            'look_for_keys': False,
        }
        if auth_type == credential.AuthType.SSH_KEY:
            connect_kwargs['pkey'] = paramiko.RSAKey.from_private_key(io.StringIO(credential.private_key or ''))
        else:
            connect_kwargs['password'] = credential.password

        def _factory():
            ssh_client = paramiko.SSHClient()
            ssh_client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
            ssh_client.connect(**connect_kwargs)
            return ssh_client

        ssh_client = SSH_POOL.acquire(pool_key, _factory)
        try:
            transport = ssh_client.get_transport()
            if transport is None:
                raise RuntimeError('SSH transport 不可用')
            sftp_client = paramiko.SFTPClient.from_transport(
                transport,
                window_size=TRANSFER_SFTP_WINDOW_SIZE,
                max_packet_size=TRANSFER_SFTP_MAX_PACKET_SIZE,
            )
            return ssh_client, sftp_client, pool_key
        except Exception:
            SSH_POOL.discard(ssh_client)
            raise

    @staticmethod
    def _request_value(request, key):
        value = request.POST.get(key) if hasattr(request, 'POST') else None
        if value is not None:
            return value
        if request.body:
            try:
                body = json.loads(request.body.decode('utf-8'))
                if isinstance(body, dict):
                    value = body.get(key)
                    if value is not None:
                        return value
            except Exception:
                pass
        return request.GET.get(key)


class TransferDownloadView(TransferBaseView):
    def get(self, request):
        trace_id = str(uuid.uuid4())[:8]
        started_at = time.perf_counter()
        stage_ms = {}

        ticket = (request.GET.get('ticket') or '').strip()
        if not ticket:
            return self._stream_error('missing ticket', status=401)

        try:
            payload = parse_download_ticket(ticket)
        except jwt.ExpiredSignatureError:
            return self._stream_error('ticket expired', status=401)
        except Exception:
            return self._stream_error('invalid ticket', status=401)

        host_id = payload.get('hid')
        remote_path = str(payload.get('path') or '').strip()
        if not host_id or not remote_path:
            return self._stream_error('invalid ticket payload', status=401)

        host = Host.objects.filter(id=host_id).first()
        if not host:
            return self._stream_error('host not found', status=404)

        logger.warning(
            '[transfer-download:%s] start host=%s path=%s stream_chunk=%s prefetch=%s sftp_window=%s sftp_packet=%s server=%s',
            trace_id,
            host_id,
            remote_path,
            TRANSFER_STREAM_CHUNK_BYTES,
            TRANSFER_SFTP_PREFETCH_REQUESTS,
            TRANSFER_SFTP_WINDOW_SIZE,
            TRANSFER_SFTP_MAX_PACKET_SIZE,
            request.META.get('SERVER_SOFTWARE', '-'),
        )

        ssh_client = None
        sftp_client = None
        pool_key = None
        remote_file = None
        try:
            t_connect = time.perf_counter()
            logger.warning('[transfer-download:%s] connecting_sftp host=%s', trace_id, host_id)
            ssh_client, sftp_client, pool_key = self._connect_sftp(host)
            stage_ms['connect'] = int((time.perf_counter() - t_connect) * 1000)

            t_stat = time.perf_counter()
            logger.warning('[transfer-download:%s] stat_remote path=%s', trace_id, remote_path)
            target_path = sftp_client.normalize(remote_path)
            target_stat = sftp_client.lstat(target_path)
            stage_ms['stat'] = int((time.perf_counter() - t_stat) * 1000)
            if stat.S_ISDIR(target_stat.st_mode):
                range_header = request.headers.get('Range') or request.META.get('HTTP_RANGE')
                if range_header:
                    self._release_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=False)
                    return self._stream_error('directory download does not support range', status=400)

                # Some SSH servers allow only one session channel per connection (MaxSessions=1).
                # Close SFTP channel before opening an exec channel for tar streaming.
                try:
                    sftp_client.close()
                except Exception:
                    pass
                sftp_client = None

                normalized_dir = target_path.rstrip('/') or '/'
                parent_dir = posixpath.dirname(normalized_dir) or '/'
                dir_name = posixpath.basename(normalized_dir)
                if not dir_name:
                    dir_name = 'root'
                archive_name = f'{dir_name}.tar.gz'

                t_open = time.perf_counter()
                tar_command = f"tar -czf - -C {shlex.quote(parent_dir)} {shlex.quote(dir_name)}"
                logger.warning('[transfer-download:%s] opening_remote_archive command=%s', trace_id, tar_command)
                stdin, stdout, stderr = ssh_client.exec_command(tar_command, get_pty=False)
                try:
                    stdin.close()
                except Exception:
                    pass
                stage_ms['open'] = int((time.perf_counter() - t_open) * 1000)

                def stream_directory():
                    sent_bytes = 0
                    transfer_started = time.perf_counter()
                    last_progress_log_at = transfer_started
                    broken = False
                    try:
                        while True:
                            chunk = stdout.read(TRANSFER_STREAM_CHUNK_BYTES)
                            if not chunk:
                                break
                            sent_bytes += len(chunk)
                            now = time.perf_counter()
                            if now - last_progress_log_at >= TRANSFER_STREAM_PROGRESS_LOG_SECONDS:
                                elapsed = max(now - transfer_started, 0.001)
                                speed_mb = (sent_bytes / 1024 / 1024) / elapsed
                                logger.warning(
                                    '[transfer-download:%s] progress archive sent=%s avg_speed=%.2fMB/s',
                                    trace_id,
                                    sent_bytes,
                                    speed_mb,
                                )
                                last_progress_log_at = now
                            yield chunk
                    except Exception:
                        broken = True
                        raise
                    finally:
                        exit_status = -1
                        try:
                            exit_status = stdout.channel.recv_exit_status()
                        except Exception:
                            broken = True
                        stderr_text = ''
                        try:
                            stderr_text = stderr.read().decode('utf-8', errors='ignore').strip()
                        except Exception:
                            pass
                        if exit_status != 0:
                            broken = True
                            logger.warning(
                                '[transfer-download:%s] archive command failed status=%s stderr=%s',
                                trace_id,
                                exit_status,
                                stderr_text,
                            )
                        elapsed = max(time.perf_counter() - transfer_started, 0.001)
                        speed_mb = (sent_bytes / 1024 / 1024) / elapsed
                        logger.warning(
                            '[transfer-download:%s] archive completed sent=%s bytes elapsed=%.2fs avg_speed=%.2fMB/s',
                            trace_id,
                            sent_bytes,
                            elapsed,
                            speed_mb,
                        )
                        try:
                            stdout.close()
                        except Exception:
                            pass
                        try:
                            stderr.close()
                        except Exception:
                            pass
                        self._release_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=broken)

                response = StreamingHttpResponse(stream_directory(), status=200, content_type='application/gzip')
                response['Content-Disposition'] = f"attachment; filename*=UTF-8''{quote(archive_name)}"
                response['X-Transfer-Trace-Id'] = trace_id
                response['X-Accel-Buffering'] = 'no'
                response['Cache-Control'] = 'no-cache'
                return response

            file_size = int(target_stat.st_size or 0)
            start = 0
            end = max(file_size - 1, 0)
            status_code = 200

            range_header = request.headers.get('Range') or request.META.get('HTTP_RANGE')
            if range_header:
                match = re.match(r'bytes=(\d*)-(\d*)$', range_header.strip())
                if not match:
                    response = HttpResponse(status=416)
                    response['Content-Range'] = f'bytes */{file_size}'
                    self._release_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=False)
                    return response
                start_text, end_text = match.groups()
                if start_text == '' and end_text == '':
                    response = HttpResponse(status=416)
                    response['Content-Range'] = f'bytes */{file_size}'
                    self._release_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=False)
                    return response
                if start_text == '':
                    suffix_length = int(end_text)
                    start = max(file_size - suffix_length, 0)
                    end = max(file_size - 1, 0)
                else:
                    start = int(start_text)
                    end = int(end_text) if end_text else max(file_size - 1, 0)
                if start >= file_size or end < start:
                    response = HttpResponse(status=416)
                    response['Content-Range'] = f'bytes */{file_size}'
                    self._release_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=False)
                    return response
                end = min(end, file_size - 1)
                status_code = 206

            t_open = time.perf_counter()
            logger.warning('[transfer-download:%s] opening_remote_file path=%s', trace_id, target_path)
            # Larger SFTP read buffer helps reduce protocol round-trips on high-bandwidth links.
            remote_file = sftp_client.file(target_path, 'rb', bufsize=TRANSFER_STREAM_CHUNK_BYTES)
            if start > 0:
                remote_file.seek(start)
            remote_file.prefetch(max_concurrent_requests=TRANSFER_SFTP_PREFETCH_REQUESTS)
            stage_ms['open'] = int((time.perf_counter() - t_open) * 1000)

            def stream_file():
                remaining = (end - start + 1) if file_size > 0 else 0
                total_to_send = remaining
                sent_bytes = 0
                transfer_started = time.perf_counter()
                last_progress_log_at = transfer_started
                try:
                    first_chunk = True
                    first_chunk_started = time.perf_counter()
                    while remaining > 0:
                        # Send a smaller first chunk to reduce time-to-first-byte on slow links/proxies.
                        chunk_limit = TRANSFER_STREAM_FIRST_CHUNK_BYTES if first_chunk else TRANSFER_STREAM_CHUNK_BYTES
                        chunk = remote_file.read(min(chunk_limit, remaining))
                        if not chunk:
                            break
                        remaining -= len(chunk)
                        sent_bytes += len(chunk)
                        if first_chunk:
                            stage_ms['first_chunk'] = int((time.perf_counter() - first_chunk_started) * 1000)
                            logger.warning(
                                '[transfer-download:%s] host=%s size=%s range=%s-%s connect=%sms stat=%sms open=%sms first_chunk=%sms first_bytes=%s',
                                trace_id,
                                host_id,
                                file_size,
                                start,
                                end,
                                stage_ms.get('connect', -1),
                                stage_ms.get('stat', -1),
                                stage_ms.get('open', -1),
                                stage_ms.get('first_chunk', -1),
                                len(chunk),
                            )
                        now = time.perf_counter()
                        if now - last_progress_log_at >= TRANSFER_STREAM_PROGRESS_LOG_SECONDS:
                            elapsed = max(now - transfer_started, 0.001)
                            speed_mb = (sent_bytes / 1024 / 1024) / elapsed
                            progress_pct = (sent_bytes / total_to_send * 100.0) if total_to_send > 0 else 100.0
                            logger.warning(
                                '[transfer-download:%s] progress sent=%s/%s (%.2f%%) avg_speed=%.2fMB/s',
                                trace_id,
                                sent_bytes,
                                total_to_send,
                                progress_pct,
                                speed_mb,
                            )
                            last_progress_log_at = now
                        first_chunk = False
                        yield chunk
                finally:
                    elapsed = max(time.perf_counter() - transfer_started, 0.001)
                    speed_mb = (sent_bytes / 1024 / 1024) / elapsed
                    logger.warning(
                        '[transfer-download:%s] completed sent=%s bytes elapsed=%.2fs avg_speed=%.2fMB/s',
                        trace_id,
                        sent_bytes,
                        elapsed,
                        speed_mb,
                    )
                    try:
                        remote_file.close()
                    except Exception:
                        pass
                    self._release_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=False)

            file_name = target_path.split('/')[-1] or 'download.bin'
            response = StreamingHttpResponse(stream_file(), status=status_code, content_type='application/octet-stream')
            response['Content-Disposition'] = f"attachment; filename*=UTF-8''{quote(file_name)}"
            response['Accept-Ranges'] = 'bytes'
            response['Content-Length'] = str(max(end - start + 1, 0))
            response['X-Transfer-Trace-Id'] = trace_id
            # Avoid reverse-proxy buffering so browser can receive chunks continuously.
            response['X-Accel-Buffering'] = 'no'
            response['Cache-Control'] = 'no-cache'
            if status_code == 206:
                response['Content-Range'] = f'bytes {start}-{end}/{file_size}'
            return response
        except Exception as exc:
            if remote_file is not None:
                try:
                    remote_file.close()
                except Exception:
                    pass
            logger.exception(
                '[transfer-download:%s] failed host=%s path=%s connect=%sms stat=%sms open=%sms total=%sms err=%s',
                trace_id,
                host_id,
                remote_path,
                stage_ms.get('connect', -1),
                stage_ms.get('stat', -1),
                stage_ms.get('open', -1),
                int((time.perf_counter() - started_at) * 1000),
                exc,
            )
            self._release_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=True)
            return self._stream_error(str(exc), status=400)


class TransferUploadChunkView(TransferBaseView):
    def post(self, request):
        ticket = str(self._request_value(request, 'ticket') or '').strip()
        upload_id = _safe_upload_id(self._request_value(request, 'upload_id'))
        upload_chunk = request.FILES.get('chunk')
        if not ticket:
            return self._error('ticket 不能为空')
        if not upload_id:
            return self._error('upload_id 非法')
        if upload_chunk is None:
            return self._error('缺少上传分片')
        try:
            chunk_index = int(self._request_value(request, 'chunk_index') or 0)
            total_chunks = int(self._request_value(request, 'total_chunks') or 0)
        except (TypeError, ValueError):
            return self._error('chunk_index/total_chunks 非法')
        if chunk_index < 0 or total_chunks <= 0 or chunk_index >= total_chunks:
            return self._error('chunk_index/total_chunks 超出范围')
        try:
            payload = parse_upload_ticket(ticket)
        except jwt.ExpiredSignatureError:
            return self._error('ticket 已过期', code=401)
        except Exception:
            return self._error('ticket 非法', code=401)

        host_id = payload.get('hid')
        target_path = str(payload.get('path') or '').strip()
        file_name = str(payload.get('name') or '').strip()
        if not host_id or not target_path or not file_name or '/' in file_name:
            return self._error('ticket 数据无效', code=401)
        host = Host.objects.filter(id=host_id).first()
        if not host:
            return self._error('主机不存在', code=404)

        ssh_client = None
        sftp_client = None
        pool_key = None
        broken = False
        try:
            ssh_client, sftp_client, pool_key = self._connect_sftp(host)
            normalized_dir = sftp_client.normalize(target_path)
            remote_target_path = posixpath.join(normalized_dir.rstrip('/'), file_name) if normalized_dir != '/' else f'/{file_name}'
            temp_name = f'.{file_name}.{upload_id}.part'
            remote_temp_path = posixpath.join(normalized_dir.rstrip('/'), temp_name) if normalized_dir != '/' else f'/{temp_name}'

            write_mode = 'wb' if chunk_index == 0 else 'ab'
            with sftp_client.file(remote_temp_path, write_mode) as remote_file:
                for chunk in upload_chunk.chunks():
                    remote_file.write(chunk)

            done = chunk_index + 1 == total_chunks
            if done:
                try:
                    existing_stat = sftp_client.lstat(remote_target_path)
                    if not stat.S_ISDIR(existing_stat.st_mode):
                        sftp_client.remove(remote_target_path)
                except Exception:
                    pass
                sftp_client.rename(remote_temp_path, remote_target_path)
                return self._ok({
                    'done': True,
                    'path': remote_target_path,
                    'name': file_name,
                    'upload_id': upload_id,
                    'uploaded_chunks': chunk_index + 1,
                    'total_chunks': total_chunks,
                })

            return self._ok({
                'done': False,
                'upload_id': upload_id,
                'uploaded_chunks': chunk_index + 1,
                'total_chunks': total_chunks,
            })
        except Exception as exc:
            broken = True
            return self._error(str(exc))
        finally:
            self._release_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=broken)


class TransferUploadCancelView(TransferBaseView):
    def post(self, request):
        ticket = str(self._request_value(request, 'ticket') or '').strip()
        upload_id = _safe_upload_id(self._request_value(request, 'upload_id'))
        if not ticket:
            return self._error('ticket 不能为空')
        if not upload_id:
            return self._error('upload_id 非法')
        try:
            payload = parse_upload_ticket(ticket)
        except jwt.ExpiredSignatureError:
            return self._error('ticket 已过期', code=401)
        except Exception:
            return self._error('ticket 非法', code=401)

        host_id = payload.get('hid')
        target_path = str(payload.get('path') or '').strip()
        file_name = str(payload.get('name') or '').strip()
        if not host_id or not target_path or not file_name or '/' in file_name:
            return self._error('ticket 数据无效', code=401)
        host = Host.objects.filter(id=host_id).first()
        if not host:
            return self._error('主机不存在', code=404)

        ssh_client = None
        sftp_client = None
        pool_key = None
        broken = False
        try:
            ssh_client, sftp_client, pool_key = self._connect_sftp(host)
            normalized_dir = sftp_client.normalize(target_path)
            temp_name = f'.{file_name}.{upload_id}.part'
            remote_temp_path = posixpath.join(normalized_dir.rstrip('/'), temp_name) if normalized_dir != '/' else f'/{temp_name}'
            try:
                sftp_client.remove(remote_temp_path)
            except Exception:
                pass
            return self._ok({'upload_id': upload_id, 'canceled': True})
        except Exception as exc:
            broken = True
            return self._error(str(exc))
        finally:
            self._release_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=broken)


class TransferUploadStatusView(TransferBaseView):
    def get(self, request):
        ticket = str(request.GET.get('ticket') or '').strip()
        upload_id = _safe_upload_id(request.GET.get('upload_id'))
        try:
            chunk_size = int(request.GET.get('chunk_size') or (8 * 1024 * 1024))
        except (TypeError, ValueError):
            return self._error('chunk_size 非法')
        if not ticket:
            return self._error('ticket 不能为空')
        if not upload_id:
            return self._error('upload_id 非法')
        if chunk_size <= 0:
            return self._error('chunk_size 必须大于 0')
        try:
            payload = parse_upload_ticket(ticket)
        except jwt.ExpiredSignatureError:
            return self._error('ticket 已过期', code=401)
        except Exception:
            return self._error('ticket 非法', code=401)

        host_id = payload.get('hid')
        target_path = str(payload.get('path') or '').strip()
        file_name = str(payload.get('name') or '').strip()
        if not host_id or not target_path or not file_name or '/' in file_name:
            return self._error('ticket 数据无效', code=401)
        host = Host.objects.filter(id=host_id).first()
        if not host:
            return self._error('主机不存在', code=404)

        ssh_client = None
        sftp_client = None
        pool_key = None
        broken = False
        try:
            ssh_client, sftp_client, pool_key = self._connect_sftp(host)
            normalized_dir = sftp_client.normalize(target_path)
            temp_name = f'.{file_name}.{upload_id}.part'
            remote_temp_path = posixpath.join(normalized_dir.rstrip('/'), temp_name) if normalized_dir != '/' else f'/{temp_name}'

            exists = False
            uploaded_size = 0
            try:
                temp_stat = sftp_client.lstat(remote_temp_path)
                if not stat.S_ISDIR(temp_stat.st_mode):
                    exists = True
                    uploaded_size = int(temp_stat.st_size or 0)
            except Exception:
                exists = False
                uploaded_size = 0

            uploaded_chunks = uploaded_size // chunk_size
            return self._ok({
                'upload_id': upload_id,
                'filename': file_name,
                'path': normalized_dir,
                'exists': exists,
                'uploaded_size': uploaded_size,
                'uploaded_chunks': uploaded_chunks,
                'next_chunk_index': uploaded_chunks,
                'chunk_size': chunk_size,
            })
        except Exception as exc:
            broken = True
            return self._error(str(exc))
        finally:
            self._release_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=broken)
