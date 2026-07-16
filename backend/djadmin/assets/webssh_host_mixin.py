import io
import importlib
import logging
import os
import posixpath
import re
import stat
import tempfile
import time
import warnings
from hashlib import sha256
from typing import Any, cast
from urllib.parse import quote

from asgiref.sync import async_to_sync
from cryptography.utils import CryptographyDeprecationWarning
from django.conf import settings
from django.http import HttpResponse, StreamingHttpResponse
from rest_framework.decorators import action

from djadmin.utils import Response_200, Response_error_str
from user.utils import getCurrentUser

from .connection_pool import SSHConnectionPool
from .models import HostCredential, WebSSHSessionLog
from .serializer import WebSSHSessionLogSerializer
from .webssh_runtime import WebSSHRuntimeRegistry

warnings.filterwarnings(
    'ignore',
    message='.*TripleDES has been moved to cryptography\\.hazmat\\.decrepit\\.ciphers\\.algorithms\\.TripleDES.*',
    category=CryptographyDeprecationWarning,
)

try:
    import paramiko
except ImportError:  # pragma: no cover
    paramiko = None

try:
    asyncssh = importlib.import_module('asyncssh')
except ImportError:  # pragma: no cover
    asyncssh = None


TRANSFER_STREAM_FIRST_CHUNK_BYTES = max(int(getattr(settings, 'TRANSFER_STREAM_FIRST_CHUNK_BYTES', 256 * 1024)), 64 * 1024)
TRANSFER_STREAM_CHUNK_BYTES = max(int(getattr(settings, 'TRANSFER_STREAM_CHUNK_BYTES', 8 * 1024 * 1024)), 512 * 1024)
TRANSFER_SFTP_WINDOW_SIZE = max(int(getattr(settings, 'TRANSFER_SFTP_WINDOW_SIZE', 8 * 1024 * 1024)), 1024 * 1024)
TRANSFER_SFTP_MAX_PACKET_SIZE = max(int(getattr(settings, 'TRANSFER_SFTP_MAX_PACKET_SIZE', 256 * 1024)), 32 * 1024)

ASSETS_SSH_POOL = SSHConnectionPool(
    max_per_key=int(getattr(settings, 'TRANSFER_SSH_POOL_MAX_PER_KEY', 2)),
    idle_seconds=int(getattr(settings, 'TRANSFER_SSH_POOL_IDLE_SECONDS', 120)),
)
logger = logging.getLogger(__name__)


class WebSSHHostMixin:
    @staticmethod
    def _get_host_connection_port(host, credential):
        if host.port:
            return host.port
        if credential and credential.port:
            return credential.port
        return 22

    def _get_default_credential(self, host):
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

    def _build_ssh_connect_kwargs(self, host, credential, paramiko_module):
        port = self._get_host_connection_port(host, credential)
        auth_type = credential.auth_type
        auth_secret = credential.private_key or '' if auth_type == credential.AuthType.SSH_KEY else credential.password or ''
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
            connect_kwargs['pkey'] = paramiko_module.RSAKey.from_private_key(io.StringIO(credential.private_key or ''))
        elif auth_type == credential.AuthType.PASSWORD:
            connect_kwargs['password'] = credential.password
        else:
            raise ValueError('不支持的凭证类型')
        return port, auth_type, auth_secret, connect_kwargs

    def _connect_sftp_for_stream_download(self, host):
        if paramiko is None:
            raise RuntimeError('paramiko 未安装，无法执行文件管理操作')
        paramiko_module = paramiko

        credential = self._get_default_credential(host)
        port, auth_type, auth_secret, connect_kwargs = self._build_ssh_connect_kwargs(host, credential, paramiko_module)
        auth_fingerprint = sha256(str(auth_secret).encode('utf-8')).hexdigest()
        pool_key = f'{host.id}:{host.ip}:{port}:{credential.username}:{auth_type}:{auth_fingerprint}'

        def _factory():
            ssh_client = paramiko_module.SSHClient()
            ssh_client.set_missing_host_key_policy(paramiko_module.AutoAddPolicy())
            ssh_client.connect(**connect_kwargs)
            return ssh_client

        acquire_started = time.perf_counter()
        ssh_client = ASSETS_SSH_POOL.acquire(pool_key, _factory)
        acquire_ms = int((time.perf_counter() - acquire_started) * 1000)
        transport = ssh_client.get_transport()
        if transport is None:
            ASSETS_SSH_POOL.discard(ssh_client)
            raise RuntimeError('SSH transport 不可用')
        try:
            sftp_client = paramiko_module.SFTPClient.from_transport(
                transport,
                window_size=TRANSFER_SFTP_WINDOW_SIZE,
                max_packet_size=TRANSFER_SFTP_MAX_PACKET_SIZE,
            )
            logger.warning(
                '[assets-direct-download] sftp_acquire host=%s port=%s user=%s elapsed=%sms',
                host.id,
                port,
                credential.username,
                acquire_ms,
            )
            return ssh_client, sftp_client, pool_key
        except Exception:
            ASSETS_SSH_POOL.discard(ssh_client)
            raise

    def _build_asyncssh_connect_kwargs(self, host, credential):
        port = self._get_host_connection_port(host, credential)
        connect_kwargs = {
            'host': host.ip,
            'port': port,
            'username': credential.username,
            # Keep parity with previous behavior which didn't enforce host key pinning.
            'known_hosts': None,
        }
        if credential.auth_type == credential.AuthType.SSH_KEY:
            connect_kwargs['client_keys'] = [str(credential.private_key or '').encode('utf-8')]
        elif credential.auth_type == credential.AuthType.PASSWORD:
            connect_kwargs['password'] = credential.password
        else:
            raise ValueError('不支持的凭证类型')
        return connect_kwargs

    async def _download_remote_file_via_asyncssh(self, connect_kwargs, remote_path, local_path):
        if asyncssh is None:
            raise RuntimeError('asyncssh 未安装，无法执行下载操作')

        async with asyncssh.connect(**connect_kwargs) as conn:
            async with conn.start_sftp_client() as sftp_client:
                target_path = str(await sftp_client.realpath(remote_path))
                target_stat = await sftp_client.stat(target_path)
                permissions = int(getattr(target_stat, 'permissions', 0) or 0)
                if stat.S_ISDIR(permissions):
                    raise ValueError('目录下载功能已关闭，请改为逐个下载文件')

                await sftp_client.get(target_path, local_path)
                file_size = int(getattr(target_stat, 'size', 0) or 0)
                return target_path, file_size

    async def _upload_local_file_via_asyncssh(self, connect_kwargs, target_path, file_name, local_path):
        if asyncssh is None:
            raise RuntimeError('asyncssh 未安装，无法执行上传操作')

        async with asyncssh.connect(**connect_kwargs) as conn:
            async with conn.start_sftp_client() as sftp_client:
                normalized_dir = str(await sftp_client.realpath(target_path))
                remote_target_path = posixpath.join(normalized_dir.rstrip('/'), file_name) if normalized_dir != '/' else f'/{file_name}'
                temp_name = f'.{file_name}.uploading.part'
                remote_temp_path = posixpath.join(normalized_dir.rstrip('/'), temp_name) if normalized_dir != '/' else f'/{temp_name}'

                # Use AsyncSSH native transfer path to reduce Python-level per-chunk write overhead.
                await sftp_client.put(local_path, remote_temp_path)

                try:
                    existing_stat = await sftp_client.stat(remote_target_path)
                    permissions = int(getattr(existing_stat, 'permissions', 0) or 0)
                    if not stat.S_ISDIR(permissions):
                        await sftp_client.remove(remote_target_path)
                except Exception:
                    pass

                await sftp_client.rename(remote_temp_path, remote_target_path)
                return remote_target_path

    @staticmethod
    def _remove_local_temp_file(path):
        if not path:
            return
        try:
            os.remove(path)
        except FileNotFoundError:
            pass
        except Exception:
            pass

    @staticmethod
    def _release_stream_sftp(ssh_client, sftp_client, pool_key=None, broken=False):
        if sftp_client is not None:
            try:
                sftp_client.close()
            except Exception:
                pass
        if ssh_client is not None:
            if broken or not pool_key:
                ASSETS_SSH_POOL.discard(ssh_client)
            else:
                ASSETS_SSH_POOL.release(pool_key, ssh_client)

    @staticmethod
    def _range_not_satisfiable_response(file_size):
        response = HttpResponse(status=416)
        response['Content-Range'] = f'bytes */{file_size}'
        return response

    @staticmethod
    def _parse_download_range(range_header, file_size):
        start = 0
        # 对于空文件（file_size=0），end应为-1，这样remaining=(−1−0+1)=0，Content-Length正确为0
        end = file_size - 1 if file_size > 0 else -1
        status_code = 200
        if not range_header:
            return start, end, status_code

        match = re.match(r'bytes=(\d*)-(\d*)$', str(range_header).strip())
        if not match:
            raise ValueError('invalid_range')

        start_text, end_text = match.groups()
        if start_text == '' and end_text == '':
            raise ValueError('invalid_range')

        if start_text == '':
            suffix_length = int(end_text)
            start = max(file_size - suffix_length, 0)
            # 保持与无Range请求时的一致性：空文件时end=-1
            end = file_size - 1 if file_size > 0 else -1
        else:
            start = int(start_text)
            end = int(end_text) if end_text else (file_size - 1 if file_size > 0 else -1)

        # 对于空文件，不进行范围验证；对于普通文件，验证start/end有效性
        if file_size > 0:
            if start >= file_size or end < start:
                raise ValueError('invalid_range')

        end = min(end, file_size - 1)
        return start, end, 206

    def _remove_remote_path(self, sftp_client, remote_path):
        entry = sftp_client.lstat(remote_path)
        if stat.S_ISDIR(entry.st_mode):
            for child in sftp_client.listdir_attr(remote_path):
                child_path = posixpath.join(remote_path.rstrip('/'), child.filename) if remote_path != '/' else f'/{child.filename}'
                self._remove_remote_path(sftp_client, child_path)
            sftp_client.rmdir(remote_path)
            return
        sftp_client.remove(remote_path)

    def _force_close_webssh_sessions_for_hosts(self, host_ids):
        normalized_host_ids = []
        for host_id in host_ids or []:
            try:
                value = int(host_id)
            except (TypeError, ValueError):
                continue
            if value > 0:
                normalized_host_ids.append(value)

        if not normalized_host_ids:
            return 0

        return async_to_sync(WebSSHRuntimeRegistry.close_active_sessions_for_hosts)(
            normalized_host_ids,
            message='关联主机已删除，连接已关闭',
            close_code=4410,
        )

    @staticmethod
    def _get_default_credential_id_for_host(host):
        relation = HostCredential.objects.filter(host=host, is_default=True).values('credential_id').first()
        if not relation:
            return None
        return relation.get('credential_id')

    def _guard_webssh_file_access(self, request, host):
        active_session_ids = WebSSHRuntimeRegistry.get_active_session_ids_for_host(host.id)

        user_info = getCurrentUser(request) or {}
        user_id = user_info.get('user_id')
        username = str(user_info.get('username') or '').strip()

        queryset = WebSSHSessionLog.objects.filter(
            host=host,
            status=WebSSHSessionLog.Status.CONNECTED,
            end_time__isnull=True,
        )

        if active_session_ids:
            queryset = queryset.filter(id__in=active_session_ids)

        if user_id not in [None, '', 0, '0']:
            queryset = queryset.filter(user_id=user_id)
        elif username:
            queryset = queryset.filter(username=username)

        if not queryset.exists():
            return Response_error_str('WebSSH 已离线，请先连接终端后再操作文件', code=400)

        return None

    @action(detail=True, methods=['get'], url_path='webssh-sessions')
    def webssh_sessions(self, request, id=None):
        host = cast(Any, self).get_object()
        queryset = WebSSHSessionLog.objects.filter(host=host).order_by('-start_time')

        username = request.query_params.get('username')
        status = request.query_params.get('status')
        if username:
            queryset = queryset.filter(username__icontains=username)
        if status in {
            WebSSHSessionLog.Status.CONNECTED,
            WebSSHSessionLog.Status.CLOSED,
            WebSSHSessionLog.Status.FAILED,
        }:
            queryset = queryset.filter(status=status)

        page = cast(Any, self).paginate_queryset(queryset)
        serializer = WebSSHSessionLogSerializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = cast(Any, self).paginator
            return Response_200(data={
                'count': paginator.page.paginator.count,
                'results': data,
                'pageNumber': paginator.page.number,
                'pageSize': paginator.page_size,
                'totalPages': paginator.page.paginator.num_pages,
                'next': paginator.get_next_link(),
                'previous': paginator.get_previous_link(),
            })
        return Response_200(data=data)

    @action(detail=True, methods=['get'], url_path='webssh-active-count')
    def webssh_active_count(self, request, id=None):
        host = cast(Any, self).get_object()
        return Response_200(data={
            'host_id': host.id,
            'active_count': WebSSHRuntimeRegistry.get_active_count_for_host(host.id),
        })

    @action(detail=True, methods=['get'], url_path='webssh-active-sessions')
    def webssh_active_sessions(self, request, id=None):
        host = cast(Any, self).get_object()
        active_session_ids = WebSSHRuntimeRegistry.get_active_session_ids_for_host(host.id)
        queryset = WebSSHSessionLog.objects.filter(
            host=host,
            status=WebSSHSessionLog.Status.CONNECTED,
            id__in=active_session_ids,
        ).order_by('start_time')
        sessions = [{
            'id': item.id,  # type: ignore[attr-defined]
            'username': item.username,
            'start_time': item.start_time,
        } for item in queryset]
        return Response_200(data={
            'host_id': host.id,
            'active_count': len(sessions),
            'sessions': sessions,
        })

    @action(detail=True, methods=['get'], url_path='files/list')
    def webssh_files(self, request, id=None):
        host = cast(Any, self).get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        requested_path = (request.query_params.get('path') or '.').strip()
        ssh_client = None
        sftp_client = None
        pool_key = None
        broken = False
        try:
            ssh_client, sftp_client, pool_key = self._connect_sftp_for_stream_download(host)
            assert sftp_client is not None
            current_path = sftp_client.normalize(requested_path)
            attrs = sftp_client.listdir_attr(current_path)
            entries = []
            for item in attrs:
                entry_path = (
                    posixpath.join(current_path.rstrip('/'), item.filename)
                    if current_path != '/' else f'/{item.filename}'
                )
                is_dir = stat.S_ISDIR(int(item.st_mode or 0))
                entries.append({
                    'name': item.filename,
                    'path': entry_path,
                    'is_dir': is_dir,
                    'size': None if is_dir else item.st_size,
                    'mtime': item.st_mtime,
                })
            entries.sort(key=lambda item: (not item['is_dir'], item['name'].lower()))
            normalized = current_path.rstrip('/') or '/'
            parent_path = None if normalized == '/' else (posixpath.dirname(normalized) or '/')
            return Response_200(data={
                'current_path': current_path,
                'parent_path': parent_path,
                'entries': entries,
            })
        except Exception as exc:
            broken = True
            return Response_error_str(str(exc), code=400)
        finally:
            self._release_stream_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=broken)

    @action(detail=True, methods=['get'], url_path='files/download')
    def webssh_file_download(self, request, id=None):
        host = cast(Any, self).get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        remote_path = (request.query_params.get('path') or '').strip()
        if not remote_path:
            return Response_error_str('path 不能为空', code=400)

        target_path = remote_path
        local_temp_file = None
        try:
            fd, local_temp_file = tempfile.mkstemp(prefix='webssh-download-', suffix='.tmp')
            os.close(fd)

            # ORM query must stay in sync context; async function only handles network I/O.
            credential = self._get_default_credential(host)
            connect_kwargs = self._build_asyncssh_connect_kwargs(host, credential)

            target_path, file_size = async_to_sync(self._download_remote_file_via_asyncssh)(
                connect_kwargs,
                remote_path,
                local_temp_file,
            )
            range_header = request.headers.get('Range') or request.META.get('HTTP_RANGE')
            try:
                start, end, status_code = self._parse_download_range(range_header, file_size)
            except ValueError:
                self._remove_local_temp_file(local_temp_file)
                return self._range_not_satisfiable_response(file_size)

            def stream_file():
                remaining = (end - start + 1) if file_size > 0 else 0
                first_chunk = True
                try:
                    with open(local_temp_file, 'rb', buffering=TRANSFER_STREAM_CHUNK_BYTES) as local_file:
                        if start > 0:
                            local_file.seek(start)
                        while remaining > 0:
                            chunk_limit = TRANSFER_STREAM_FIRST_CHUNK_BYTES if first_chunk else TRANSFER_STREAM_CHUNK_BYTES
                            chunk = local_file.read(min(chunk_limit, remaining))
                            if not chunk:
                                break
                            remaining -= len(chunk)
                            first_chunk = False
                            yield chunk
                finally:
                    self._remove_local_temp_file(local_temp_file)

            file_name = target_path.split('/')[-1] or 'download.bin'
            content_length = max(end - start + 1, 0)
            response = StreamingHttpResponse(stream_file(), status=status_code, content_type='application/octet-stream')
            response['Content-Disposition'] = f"attachment; filename*=UTF-8''{quote(file_name)}"
            response['Accept-Ranges'] = 'bytes'
            response['Content-Length'] = str(content_length)
            if status_code == 206:
                response['Content-Range'] = f'bytes {start}-{end}/{file_size}'
            response['X-Accel-Buffering'] = 'no'
            response['Cache-Control'] = 'no-cache'
            return response
        except Exception as exc:
            self._remove_local_temp_file(local_temp_file)
            return Response_error_str(str(exc), code=400)

    @action(detail=True, methods=['post'], url_path='files/upload/chunk')
    def webssh_file_upload_chunk(self, request, id=None):
        host = cast(Any, self).get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response

        upload_file = request.FILES.get('file')
        target_path = (request.data.get('path') or '.').strip()
        file_name = (request.data.get('filename') or '').strip()
        if upload_file is None:
            return Response_error_str('缺少上传文件', code=400)
        if not file_name:
            return Response_error_str('filename 不能为空', code=400)
        if '/' in file_name:
            return Response_error_str('filename 不能包含路径分隔符', code=400)

        local_temp_file = None
        try:
            # Keep Django upload handling in sync context, then hand over to AsyncSSH for remote transfer.
            fd, local_temp_file = tempfile.mkstemp(prefix='webssh-upload-', suffix='.part')
            with os.fdopen(fd, 'wb') as local_file:
                for chunk in upload_file.chunks(chunk_size=TRANSFER_STREAM_CHUNK_BYTES):
                    if not chunk:
                        continue
                    local_file.write(chunk)

            credential = self._get_default_credential(host)
            connect_kwargs = self._build_asyncssh_connect_kwargs(host, credential)
            remote_target_path = async_to_sync(self._upload_local_file_via_asyncssh)(
                connect_kwargs,
                target_path,
                file_name,
                local_temp_file,
            )
            return Response_200(data={
                'done': True,
                'path': remote_target_path,
                'name': file_name,
                'size': int(upload_file.size or 0),
            })
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        finally:
            self._remove_local_temp_file(local_temp_file)

    @action(detail=True, methods=['post'], url_path='files/rename')
    def webssh_file_rename(self, request, id=None):
        host = cast(Any, self).get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        remote_path = (request.data.get('path') or '').strip()
        new_name = (request.data.get('new_name') or '').strip()
        if not remote_path:
            return Response_error_str('path 不能为空', code=400)
        if not new_name:
            return Response_error_str('new_name 不能为空', code=400)
        if '/' in new_name:
            return Response_error_str('new_name 不能包含路径分隔符', code=400)
        ssh_client = None
        sftp_client = None
        pool_key = None
        broken = False
        try:
            ssh_client, sftp_client, pool_key = self._connect_sftp_for_stream_download(host)
            assert sftp_client is not None
            normalized_old_path = sftp_client.normalize(remote_path)
            new_path = posixpath.join(posixpath.dirname(normalized_old_path), new_name)
            sftp_client.rename(normalized_old_path, new_path)
            return Response_200(data={'path': new_path, 'name': new_name})
        except Exception as exc:
            broken = True
            return Response_error_str(str(exc), code=400)
        finally:
            self._release_stream_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=broken)

    @action(detail=True, methods=['delete'], url_path='files/delete')
    def webssh_file_delete(self, request, id=None):
        host = cast(Any, self).get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        remote_path = (request.data.get('path') or '').strip()
        recursive = bool(request.data.get('recursive'))
        if not remote_path:
            return Response_error_str('path 不能为空', code=400)
        ssh_client = None
        sftp_client = None
        pool_key = None
        broken = False
        try:
            ssh_client, sftp_client, pool_key = self._connect_sftp_for_stream_download(host)
            assert sftp_client is not None
            normalized_path = sftp_client.normalize(remote_path)
            target_stat = sftp_client.lstat(normalized_path)
            if stat.S_ISDIR(int(target_stat.st_mode or 0)):
                if not recursive:
                    return Response_error_str('目录删除需要 recursive=true', code=400)
                self._remove_remote_path(sftp_client, normalized_path)
            else:
                sftp_client.remove(normalized_path)
            return Response_200(data={'path': normalized_path})
        except Exception as exc:
            broken = True
            return Response_error_str(str(exc), code=400)
        finally:
            self._release_stream_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=broken)

    @action(detail=True, methods=['post'], url_path='files/create-dir')
    def webssh_file_create_dir(self, request, id=None):
        host = cast(Any, self).get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        target_path = (request.data.get('path') or '.').strip()
        name = (request.data.get('name') or '').strip()
        if not name:
            return Response_error_str('name 不能为空', code=400)
        if '/' in name:
            return Response_error_str('name 不能包含路径分隔符', code=400)
        ssh_client = None
        sftp_client = None
        pool_key = None
        broken = False
        try:
            ssh_client, sftp_client, pool_key = self._connect_sftp_for_stream_download(host)
            assert sftp_client is not None
            normalized_dir = sftp_client.normalize(target_path)
            new_dir_path = posixpath.join(normalized_dir.rstrip('/'), name) if normalized_dir != '/' else f'/{name}'
            sftp_client.mkdir(new_dir_path)
            return Response_200(data={'path': new_dir_path, 'name': name})
        except Exception as exc:
            broken = True
            return Response_error_str(str(exc), code=400)
        finally:
            self._release_stream_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=broken)

    @action(detail=True, methods=['post'], url_path='files/create-file')
    def webssh_file_create_file(self, request, id=None):
        host = cast(Any, self).get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        target_path = (request.data.get('path') or '.').strip()
        name = (request.data.get('name') or '').strip()
        if not name:
            return Response_error_str('name 不能为空', code=400)
        if '/' in name:
            return Response_error_str('name 不能包含路径分隔符', code=400)
        ssh_client = None
        sftp_client = None
        pool_key = None
        broken = False
        try:
            ssh_client, sftp_client, pool_key = self._connect_sftp_for_stream_download(host)
            assert sftp_client is not None
            normalized_dir = sftp_client.normalize(target_path)
            new_file_path = posixpath.join(normalized_dir.rstrip('/'), name) if normalized_dir != '/' else f'/{name}'
            with sftp_client.file(new_file_path, 'xb') as remote_file:
                remote_file.write(b'')
            return Response_200(data={'path': new_file_path, 'name': name})
        except Exception as exc:
            broken = True
            return Response_error_str(str(exc), code=400)
        finally:
            self._release_stream_sftp(ssh_client, sftp_client, pool_key=pool_key, broken=broken)
