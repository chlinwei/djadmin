import posixpath
import re
from typing import Any, cast
from urllib.parse import quote

from asgiref.sync import async_to_sync
from django.conf import settings
from django.http import HttpResponse, StreamingHttpResponse
from rest_framework.decorators import action

from djadmin.utils import Response_200, Response_error_str
from user.utils import getCurrentUser

from .grpc_transfer.client import AgentChannelClient, AgentGrpcTransferError
from .models import HostCredential, WebSSHSessionLog
from .serializer import WebSSHSessionLogSerializer
from .webssh_runtime import WebSSHRuntimeRegistry

# dj-agent 是 WebSSH 文件操作（list/download/upload/rename/delete/mkdir/create-file）的
# 唯一实现路径：不再保留直连 SSH/SFTP 的回退分支。原因：
#   1. 直连 SSH 分支与 gRPC 分支长期并存导致每个操作都要维护两套几乎重复的逻辑，
#      显著增加维护成本却没有换来额外能力——两条路径给前端返回的数据结构完全一致。
#   2. WebSSH 的准入本来就已经要求 host.agent_online（见 consumers.py 的
#      `_get_host_and_agent`），没有 agent 在线本来就打不开 WebSSH，SSH 直连分支
#      实际上从未在生产场景下被真正触发过。
# 因此这里彻底移除 paramiko/SSHConnectionPool 相关代码；文件操作强制要求 agent 已建立
# gRPC 会话，否则直接返回明确的错误信息（而不是静默退化到另一套实现）。
TRANSFER_STREAM_CHUNK_BYTES = max(int(getattr(settings, 'TRANSFER_STREAM_CHUNK_BYTES', 8 * 1024 * 1024)), 512 * 1024)


class WebSSHHostMixin:

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

    @staticmethod
    def _get_agent_grpc_client(host):
        # dj-agent 必须存在且已通过 gRPC 建立通道长连接，否则直接抛错——不再有
        # 任何“退回直连 SSH”的兜底路径。三层校验依次是：心跳在线 -> 绑定了 agent_id ->
        # 该 agent_id 确实已经在 REGISTRY 里建立了 gRPC Session（AgentChannelClient
        # 构造函数内部会在查不到 session 时抛 AgentGrpcTransferError）。
        if not getattr(host, 'agent_online', False):
            raise AgentGrpcTransferError('主机 Agent 离线，无法执行文件操作，请确认 dj-agent 已安装并在线')
        agent_id = str(getattr(host, 'instance_name', '') or '').strip()
        if not agent_id:
            raise AgentGrpcTransferError('主机未绑定 Agent 实例，无法执行文件操作')
        return AgentChannelClient(agent_id)


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

        try:
            grpc_client = self._get_agent_grpc_client(host)
            resp = grpc_client.list_dir(requested_path)
        except Exception as exc:
            return Response_error_str(str(exc), code=400)

        current_path = resp.current_path
        entries = []
        for item in resp.entries:
            entry_path = (
                posixpath.join(current_path.rstrip('/'), item.name)
                if current_path != '/' else f'/{item.name}'
            )
            entries.append({
                'name': item.name,
                'path': entry_path,
                'is_dir': item.is_dir,
                'size': None if item.is_dir else item.size,
                'mtime': item.mtime,
            })
        entries.sort(key=lambda item: (not item['is_dir'], item['name'].lower()))
        normalized = current_path.rstrip('/') or '/'
        parent_path = None if normalized == '/' else (posixpath.dirname(normalized) or '/')
        return Response_200(data={
            'current_path': current_path,
            'parent_path': parent_path,
            'entries': entries,
        })

    def _webssh_file_download_via_agent(self, grpc_client, remote_path, request):
        try:
            stat_resp = grpc_client.stat(remote_path)
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        if stat_resp.is_dir:
            return Response_error_str('目录下载功能已关闭，请改为逐个下载文件', code=400)

        normalized_path = stat_resp.normalized_path
        file_size = int(stat_resp.size or 0)
        file_name = posixpath.basename(normalized_path.rstrip('/')) or 'download'

        range_header = request.headers.get('Range') or request.META.get('HTTP_RANGE')
        try:
            start, end, status_code = self._parse_download_range(range_header, file_size)
        except ValueError:
            return self._range_not_satisfiable_response(file_size)

        length = (end - start + 1) if file_size > 0 else 0

        def stream_file():
            for chunk in grpc_client.read_stream(normalized_path, offset=start, length=length):
                if chunk.data:
                    yield chunk.data

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

    @action(detail=True, methods=['get'], url_path='files/download')
    def webssh_file_download(self, request, id=None):
        host = cast(Any, self).get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        remote_path = (request.query_params.get('path') or '').strip()
        if not remote_path:
            return Response_error_str('path 不能为空', code=400)

        try:
            grpc_client = self._get_agent_grpc_client(host)
        except AgentGrpcTransferError as exc:
            return Response_error_str(str(exc), code=400)
        return self._webssh_file_download_via_agent(grpc_client, remote_path, request)

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

        try:
            grpc_client = self._get_agent_grpc_client(host)
        except AgentGrpcTransferError as exc:
            return Response_error_str(str(exc), code=400)

        write_session = None
        try:
            write_session = grpc_client.open_write(target_path, file_name)
            for chunk in upload_file.chunks(chunk_size=TRANSFER_STREAM_CHUNK_BYTES):
                if chunk:
                    write_session.write_chunk(chunk)
            close_resp = write_session.close(abort=False)
            assert close_resp is not None
            return Response_200(data={
                'done': True,
                'path': close_resp.path,
                'name': file_name,
                'size': int(upload_file.size or 0),
            })
        except Exception as exc:
            if write_session is not None:
                try:
                    write_session.close(abort=True)
                except Exception:
                    pass
            return Response_error_str(str(exc), code=400)

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

        try:
            grpc_client = self._get_agent_grpc_client(host)
            resp = grpc_client.rename(remote_path, new_name)
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data={'path': resp.path, 'name': resp.name})

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

        try:
            grpc_client = self._get_agent_grpc_client(host)
            resp = grpc_client.delete(remote_path, recursive=recursive)
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data={'path': resp.path})

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

        try:
            grpc_client = self._get_agent_grpc_client(host)
            resp = grpc_client.mkdir(target_path, name)
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data={'path': resp.path, 'name': resp.name})

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

        try:
            grpc_client = self._get_agent_grpc_client(host)
            resp = grpc_client.create_file(target_path, name)
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        return Response_200(data={'path': resp.path, 'name': resp.name})
