import io
import posixpath
import re
import stat
import warnings
from urllib.parse import quote

from cryptography.utils import CryptographyDeprecationWarning
from django.http import HttpResponse, StreamingHttpResponse
from django.shortcuts import render
from djadmin.utils import Response_200, Response_error_str
from rest_framework.mixins import CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin
from rest_framework.viewsets import GenericViewSet
from rest_framework.decorators import action
from .models import *
from .serializer import *
from .tasks import collect_host_info, collect_all_hosts_info
from djadmin.utils import CustomPagination
from rest_framework.filters import OrderingFilter,SearchFilter
from django_filters.rest_framework  import DjangoFilterBackend
from djadmin.errordict import DjadminException,AssetsError
from io import TextIOWrapper
import csv
from menu.permisssion import CustomMenuPermission
from user.utils import getCurrentUser
from django.db.models import Prefetch
from .webssh_runtime import WebSSHRuntimeRegistry
from transfer.tokens import issue_download_ticket, issue_upload_ticket
from django.conf import settings

warnings.filterwarnings(
    'ignore',
    message='.*TripleDES has been moved to cryptography\\.hazmat\\.decrepit\\.ciphers\\.algorithms\\.TripleDES.*',
    category=CryptographyDeprecationWarning,
)

try:
    import paramiko
except ImportError:  # pragma: no cover
    paramiko = None


def _safe_upload_id(value):
    text = str(value or '').strip()
    if not text:
        return ''
    if not re.fullmatch(r'[A-Za-z0-9_-]{1,64}', text):
        return ''
    return text


class CredentialManage(GenericViewSet,CreateModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin,DestroyModelMixin):
    queryset =  Credential.objects.all()
    serializer_class = CredentialSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter,DjangoFilterBackend,SearchFilter)
    search_fields = ['name', 'remark'] 
    ordering_fields = [ 'name','create_time'] 
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        # view
        'list': 'assets:credentials:view',
        'retrieve': 'assets:credentials:view',
        # delete
        'destroy': 'assets:credentials:delete',
        'batch-delete': 'assets:credentials:delete',
        # update
        'partial_update': 'assets:credentials:update',
        'perform_update': 'assets:credentials:update',
        # create
        'create': 'assets:credentials:create',
        'batch-create': 'assets:credentials:create',
    }

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
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

    def retrieve(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance)
        return Response_200(data=serializer.data)

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        deleted_id = instance.id
        self.perform_destroy(instance)
        return Response_200(data={"id": deleted_id})

    # 批量删除credential
    @action(detail=False,methods=['delete'],url_path='batch-delete')
    def batchDelete(self,request):
        # 获取ID数组参数
        ids = request.data.get('ids', [])
        # 先查用户角色列表
        Credential.objects.filter(id__in=ids).delete()
        return Response_200()
    # 批量导入credential
    @action(detail=False,methods=['post'],url_path='batch-create')
    def batchCreate(self,request):
        file = request.FILES.get('file')
        if not file.name.endswith('csv'):
            raise DjadminException(AssetsError.FILE_NOT_ENDSWITH_CSV)
        decoded_file = TextIOWrapper(file.file, encoding='utf-8')
        reader = csv.DictReader(decoded_file)
        input_datas = []
        for row in reader:
            input_datas.append(row)
        serializer = self.get_serializer(data=input_datas, many=True)
        if serializer.is_valid():
            serializer.save()
            return Response_200(serializer.data)
        raise DjadminException(AssetsError.BATCH_UPLOAD_ERROR,serializer.errors)
        

class ApplicationManage(GenericViewSet,CreateModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset =  Application.objects.all()
    serializer_class = ApplicationSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter,DjangoFilterBackend,SearchFilter)
    search_fields = ['name', 'remark'] 
    ordering_fields = [ 'name','create_time'] 
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        # view
        'list': 'assets:applications:view',
        'retrieve': 'assets:applications:view',
        # delete
        'batch-delete': 'assets:applications:delete',
        # update
        'partial_update': 'assets:applications:update',
        'perform_update': 'assets:applications:update',
        # create
        'create': 'assets:applications:create',
    }

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
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

    def retrieve(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance)
        return Response_200(data=serializer.data)

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    # 批量删除Application
    @action(detail=False,methods=['delete'],url_path='batch-delete')
    def batchDelete(self,request):
        # 获取ID数组参数
        ids = request.data.get('ids', [])
        # 先查用户角色列表
        Application.objects.filter(id__in=ids).delete()
        return Response_200()


class HostGroupManage(GenericViewSet,CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset = HostGroup.objects.all()
    serializer_class = HostGroupSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'remark']
    ordering_fields = ['name', 'create_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'assets:hostgroups:view',
        'retrieve': 'assets:hostgroups:view',
        'create': 'assets:hostgroups:create',
        'destroy': 'assets:hostgroups:delete',
        'partial_update': 'assets:hostgroups:update',
        'perform_update': 'assets:hostgroups:update',
        'batch-delete': 'assets:hostgroups:delete',
        'tree': 'assets:hostgroups:view',
    }

    def get_queryset(self):
        if self.action in ['list', 'retrieve', 'tree']:
            return HostGroup.objects.select_related('parent').order_by('id')
        return HostGroup.objects.all()

    def _build_tree(self, group_data):
        nodes = {}
        roots = []

        for item in group_data:
            item['children'] = []
            nodes[item['id']] = item

        for item in group_data:
            parent_id = item.get('parent')
            if parent_id:
                parent = nodes.get(parent_id)
                if parent:
                    parent['children'].append(item)
                else:
                    roots.append(item)
            else:
                roots.append(item)

        return roots

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
            return Response_200(data={
                'count': paginator.page.paginator.count,
                'results': data,
            })
        return Response_200(data=data)

    def retrieve(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance)
        return Response_200(data=serializer.data)

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    @action(detail=False, methods=['get'], url_path='tree')
    def tree(self, request):
        queryset = self.filter_queryset(self.get_queryset())
        serializer = self.get_serializer(queryset, many=True)
        data = serializer.data
        tree_data = self._build_tree(data)
        return Response_200(data=tree_data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        from automation.models import AutomationInventory

        referenced = []
        for inventory in AutomationInventory.objects.only('id', 'name', 'selected_group_ids'):
            group_ids = [int(item) for item in (inventory.selected_group_ids or []) if str(item).isdigit()]
            if instance.id in group_ids:
                referenced.append(inventory)

        if referenced:
            names = '、'.join(item.name for item in referenced[:5])
            suffix = '' if len(referenced) <= 5 else f' 等{len(referenced)}个'
            return Response_error_str(
                f'该主机组已被 Inventory 引用，不能删除。受影响 Inventory: {names}{suffix}',
                code=400,
            )

        deleted_id = instance.id
        self.perform_destroy(instance)
        return Response_200(data={"id": deleted_id})

    @action(detail=False, methods=['delete'], url_path='batch-delete')
    def batchDelete(self, request):
        ids = request.data.get('ids', [])
        if not ids:
            return Response_200(data=[])

        from automation.models import AutomationInventory

        normalized_ids = [int(item) for item in ids if str(item).isdigit()]
        referenced_pairs = []
        for inventory in AutomationInventory.objects.only('id', 'name', 'selected_group_ids'):
            group_ids = [int(item) for item in (inventory.selected_group_ids or []) if str(item).isdigit()]
            hit_ids = sorted(set(group_ids).intersection(set(normalized_ids)))
            if hit_ids:
                referenced_pairs.append((inventory.name, hit_ids))

        if referenced_pairs:
            sample = '；'.join(f"{name} -> {','.join(str(i) for i in group_ids)}" for name, group_ids in referenced_pairs[:3])
            suffix = '' if len(referenced_pairs) <= 3 else f' 等{len(referenced_pairs)}个 Inventory'
            return Response_error_str(
                f'批量删除中包含被 Inventory 引用的主机组，操作已阻止：{sample}{suffix}',
                code=400,
            )

        deleted_count, _ = HostGroup.objects.filter(id__in=ids).delete()
        return Response_200(data={"deleted_count": deleted_count})


class HostManage(GenericViewSet,CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
    queryset = Host.objects.all()
    serializer_class = HostSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['instance_name', 'system__hostname', 'ip', 'remark']
    ordering_fields = ['instance_name', 'create_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'assets:hosts:view',
        'retrieve': 'assets:hosts:view',
        'create': 'assets:hosts:create',
        'destroy': 'assets:hosts:delete',
        'partial_update': 'assets:hosts:update',
        'perform_update': 'assets:hosts:update',
        'batch-delete': 'assets:hosts:delete',
        'webssh_sessions': 'assets:hosts:view',
        'webssh_active_count': 'assets:hosts:view',
        'webssh_active_sessions': 'assets:hosts:view',
        'webssh_files': 'assets:hosts:view',
        'webssh_file_download': 'assets:hosts:view',
        'webssh_file_download_ticket': 'assets:hosts:view',
        'webssh_file_upload_ticket': 'assets:hosts:update',
        'webssh_file_upload': 'assets:hosts:update',
        'webssh_file_upload_chunk': 'assets:hosts:update',
        'webssh_file_upload_status': 'assets:hosts:view',
        'webssh_file_upload_cancel': 'assets:hosts:update',
        'webssh_file_rename': 'assets:hosts:update',
        'webssh_file_delete': 'assets:hosts:delete',
        'webssh_file_create_dir': 'assets:hosts:update',
        'webssh_file_create_file': 'assets:hosts:update',
    }

    def _get_group_and_subgroups(self, group_id):
        """递归获取分组及其所有子分组的ID列表"""
        group_ids = [group_id]
        group = HostGroup.objects.filter(id=group_id).first()
        if group:
            children = HostGroup.objects.filter(parent_id=group_id)
            for child in children:
                group_ids.extend(self._get_group_and_subgroups(child.id))  # type: ignore[attr-defined]
        return group_ids

    def _display_host_name(self, host):
        system = getattr(host, 'system', None)
        hostname = getattr(system, 'hostname', None) if system else None
        return host.instance_name or hostname or f'Host-{host.id}'

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

    def _connect_sftp(self, host):
        if paramiko is None:
            raise RuntimeError('paramiko 未安装，无法执行文件管理操作')

        credential = self._get_default_credential(host)
        ssh_client = paramiko.SSHClient()
        ssh_client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        connect_kwargs = {
            'hostname': host.ip,
            'port': self._get_host_connection_port(host, credential),
            'username': credential.username,
            'timeout': 15,
            'banner_timeout': 15,
            'allow_agent': False,
            'look_for_keys': False,
        }

        if credential.auth_type == credential.AuthType.SSH_KEY:
            key_data = credential.private_key or ''
            connect_kwargs['pkey'] = paramiko.RSAKey.from_private_key(io.StringIO(key_data))
        elif credential.auth_type == credential.AuthType.PASSWORD:
            connect_kwargs['password'] = credential.password
        else:
            raise ValueError('不支持的凭证类型')

        ssh_client.connect(**connect_kwargs)
        return ssh_client, ssh_client.open_sftp()

    @staticmethod
    def _close_sftp(ssh_client, sftp_client):
        if sftp_client is not None:
            try:
                sftp_client.close()
            except Exception:
                pass
        if ssh_client is not None:
            try:
                ssh_client.close()
            except Exception:
                pass

    def _remove_remote_path(self, sftp_client, remote_path):
        entry = sftp_client.lstat(remote_path)
        if stat.S_ISDIR(entry.st_mode):
            for child in sftp_client.listdir_attr(remote_path):
                child_path = posixpath.join(remote_path.rstrip('/'), child.filename) if remote_path != '/' else f'/{child.filename}'
                self._remove_remote_path(sftp_client, child_path)
            sftp_client.rmdir(remote_path)
            return
        sftp_client.remove(remote_path)

    def _guard_webssh_file_access(self, request, host):
        """Require an active WebSSH session for the current user before file operations.

        Prefer in-process runtime registry; fall back to DB state for multi-process deployments.
        """
        active_session_ids = WebSSHRuntimeRegistry.get_active_session_ids_for_host(host.id)

        user_info = getCurrentUser(request) or {}
        user_id = user_info.get('user_id')
        username = str(user_info.get('username') or '').strip()

        queryset = WebSSHSessionLog.objects.filter(
            host=host,
            status=WebSSHSessionLog.Status.CONNECTED,
            end_time__isnull=True,
        )

        # Runtime registry is in-memory and process-local.
        # When available, use it for stricter real-time checks; otherwise rely on DB connected state.
        if active_session_ids:
            queryset = queryset.filter(id__in=active_session_ids)

        if user_id not in [None, '', 0, '0']:
            queryset = queryset.filter(user_id=user_id)
        elif username:
            queryset = queryset.filter(username=username)

        if not queryset.exists():
            return Response_error_str('WebSSH 已离线，请先连接终端后再操作文件', code=400)

        return None

    def get_queryset(self):
        queryset = Host.objects.select_related('group').prefetch_related(
            Prefetch('hostcredential_set', queryset=HostCredential.objects.select_related('credential').filter(is_default=True)),
            'hardware',
            'system',
            'disks',
        ).order_by('-id')
        group_id = self.request.query_params.get('group_id')  # type: ignore[union-attr]
        host_id = self.request.query_params.get('host_id')  # type: ignore[union-attr]
        instance_name = (self.request.query_params.get('instance_name') or '').strip()  # type: ignore[union-attr]
        collect_status = self.request.query_params.get('collect_status')  # type: ignore[union-attr]
        has_default_credential = (self.request.query_params.get('has_default_credential') or '').strip().lower()  # type: ignore[union-attr]
        if host_id not in [None, '', '0', 0]:
            try:
                queryset = queryset.filter(id=int(host_id))
            except (TypeError, ValueError):
                queryset = queryset.none()
        if group_id not in [None, '', '0', 0]:
            # 查询该分组及其所有子分组下的主机
            group_ids = self._get_group_and_subgroups(int(group_id))
            queryset = queryset.filter(group_id__in=group_ids)
        if instance_name:
            queryset = queryset.filter(instance_name__icontains=instance_name)
        if has_default_credential in {'true', '1', 'yes'}:
            queryset = queryset.filter(hostcredential__is_default=True)
        elif has_default_credential in {'false', '0', 'no'}:
            queryset = queryset.exclude(hostcredential__is_default=True)
        if collect_status in {
            Host.CollectStatus.UNKNOWN,
            Host.CollectStatus.SUCCESS,
            Host.CollectStatus.FAILED,
        }:
            queryset = queryset.filter(collect_status=collect_status)
        return queryset.distinct()

    def create(self, request, *args, **kwargs):
        serializer = self.get_serializer(data=request.data)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        return Response_200(data=serializer.data)

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        serializer = self.get_serializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
            return Response_200(data={
                'count': paginator.page.paginator.count,
                'results': data,
            })
        return Response_200(data=data)

    def retrieve(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance)
        return Response_200(data=serializer.data)

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        self.perform_update(serializer)
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        self.perform_update(serializer)
        return Response_200(data=serializer.data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        deleted_id = instance.id
        self.perform_destroy(instance)
        return Response_200(data={"id": deleted_id})

    @action(detail=False, methods=['delete'], url_path='batch-delete')
    def batchDelete(self, request):
        ids = request.data.get('ids', [])
        if not ids:
            return Response_200(data=[])
        deleted_count, _ = Host.objects.filter(id__in=ids).delete()
        return Response_200(data={"deleted_count": deleted_count})

    @action(detail=True, methods=['get'], url_path='webssh-sessions')
    def webssh_sessions(self, request, id=None):
        host = self.get_object()
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

        page = self.paginate_queryset(queryset)
        serializer = WebSSHSessionLogSerializer(page if page is not None else queryset, many=True)
        data = serializer.data
        if page is not None:
            paginator = self.paginator
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
        host = self.get_object()
        return Response_200(data={
            'host_id': host.id,
            'active_count': WebSSHRuntimeRegistry.get_active_count_for_host(host.id),
        })

    @action(detail=True, methods=['get'], url_path='webssh-active-sessions')
    def webssh_active_sessions(self, request, id=None):
        host = self.get_object()
        active_session_ids = WebSSHRuntimeRegistry.get_active_session_ids_for_host(host.id)
        queryset = WebSSHSessionLog.objects.filter(
            host=host,
            status=WebSSHSessionLog.Status.CONNECTED,
            id__in=active_session_ids,
        ).order_by('start_time')
        sessions = [{
            'id': item.id,
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
        host = self.get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        requested_path = (request.query_params.get('path') or '.').strip()  # type: ignore[union-attr]
        ssh_client = None
        sftp_client = None
        try:
            ssh_client, sftp_client = self._connect_sftp(host)
            current_path = sftp_client.normalize(requested_path)
            attrs = sftp_client.listdir_attr(current_path)
            entries = []
            for item in attrs:
                entry_path = (
                    posixpath.join(current_path.rstrip('/'), item.filename)
                    if current_path != '/' else f'/{item.filename}'
                )
                is_dir = stat.S_ISDIR(item.st_mode)
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
            return Response_error_str(str(exc), code=400)
        finally:
            self._close_sftp(ssh_client, sftp_client)

    @action(detail=True, methods=['get'], url_path='files/download')
    def webssh_file_download(self, request, id=None):
        host = self.get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        remote_path = (request.query_params.get('path') or '').strip()  # type: ignore[union-attr]
        if not remote_path:
            return Response_error_str('path 不能为空', code=400)
        ssh_client = None
        sftp_client = None
        remote_file = None
        try:
            ssh_client, sftp_client = self._connect_sftp(host)
            target_path = sftp_client.normalize(remote_path)
            target_stat = sftp_client.lstat(target_path)
            if stat.S_ISDIR(target_stat.st_mode):
                return Response_error_str('目录不支持直接下载，请先进入目录选择文件', code=400)

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
                    return response
                start_text, end_text = match.groups()
                if start_text == '' and end_text == '':
                    response = HttpResponse(status=416)
                    response['Content-Range'] = f'bytes */{file_size}'
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
                    return response
                end = min(end, file_size - 1)
                status_code = 206

            remote_file = sftp_client.file(target_path, 'rb')
            if start > 0:
                remote_file.seek(start)

            def stream_file():
                remaining = (end - start + 1) if file_size > 0 else 0
                try:
                    while remaining > 0:
                        chunk = remote_file.read(min(1024 * 1024, remaining))
                        if not chunk:
                            break
                        remaining -= len(chunk)
                        yield chunk
                finally:
                    try:
                        remote_file.close()
                    except Exception:
                        pass
                    self._close_sftp(ssh_client, sftp_client)

            file_name = posixpath.basename(target_path) or 'download.bin'
            response = StreamingHttpResponse(stream_file(), status=status_code, content_type='application/octet-stream')
            response['Content-Disposition'] = f"attachment; filename*=UTF-8''{quote(file_name)}"
            response['Accept-Ranges'] = 'bytes'
            response['Content-Length'] = str(max(end - start + 1, 0))
            if status_code == 206:
                response['Content-Range'] = f'bytes {start}-{end}/{file_size}'
            return response
        except Exception as exc:
            if remote_file is not None:
                try:
                    remote_file.close()
                except Exception:
                    pass
            self._close_sftp(ssh_client, sftp_client)
            return Response_error_str(str(exc), code=400)

    @action(detail=True, methods=['post'], url_path='files/download-ticket')
    def webssh_file_download_ticket(self, request, id=None):
        host = self.get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        remote_path = (request.data.get('path') or '').strip()
        if not remote_path:
            return Response_error_str('path 不能为空', code=400)
        user_info = getCurrentUser(request)
        try:
            token = issue_download_ticket(
                user_id=user_info.get('user_id'),
                host_id=host.id,
                remote_path=remote_path,
            )
            transfer_base_url = str(getattr(settings, 'TRANSFER_SERVICE_BASE_URL', '')).rstrip('/')
            return Response_200(data={
                'ticket': token,
                'download_url': f'{transfer_base_url}/transfer/download/?ticket={quote(token)}' if transfer_base_url else '',
                'expires_in': int(getattr(settings, 'TRANSFER_TICKET_EXPIRE_SECONDS', 7200)),
            })
        except Exception as exc:
            return Response_error_str(str(exc), code=400)

    @action(detail=True, methods=['post'], url_path='files/upload-ticket')
    def webssh_file_upload_ticket(self, request, id=None):
        host = self.get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        target_path = (request.data.get('path') or '.').strip()
        file_name = (request.data.get('filename') or '').strip()
        if not file_name:
            return Response_error_str('filename 不能为空', code=400)
        if '/' in file_name:
            return Response_error_str('filename 不能包含路径分隔符', code=400)
        user_info = getCurrentUser(request)
        try:
            token = issue_upload_ticket(
                user_id=user_info.get('user_id'),
                host_id=host.id,
                target_path=target_path,
                file_name=file_name,
            )
            transfer_base_url = str(getattr(settings, 'TRANSFER_SERVICE_BASE_URL', '')).rstrip('/')
            return Response_200(data={
                'ticket': token,
                'upload_chunk_url': f'{transfer_base_url}/transfer/upload/chunk/' if transfer_base_url else '',
                'upload_status_url': f'{transfer_base_url}/transfer/upload/status/' if transfer_base_url else '',
                'upload_cancel_url': f'{transfer_base_url}/transfer/upload/cancel/' if transfer_base_url else '',
                'expires_in': int(getattr(settings, 'TRANSFER_TICKET_EXPIRE_SECONDS', 7200)),
            })
        except Exception as exc:
            return Response_error_str(str(exc), code=400)

    @action(detail=True, methods=['post'], url_path='files/upload')
    def webssh_file_upload(self, request, id=None):
        host = self.get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        upload_file = request.FILES.get('file')
        target_path = (request.data.get('path') or '.').strip()
        if upload_file is None:
            return Response_error_str('缺少上传文件', code=400)
        ssh_client = None
        sftp_client = None
        try:
            ssh_client, sftp_client = self._connect_sftp(host)
            normalized_dir = sftp_client.normalize(target_path)
            remote_file_path = posixpath.join(normalized_dir.rstrip('/'), upload_file.name) if normalized_dir != '/' else f'/{upload_file.name}'
            with sftp_client.file(remote_file_path, 'wb') as remote_file:
                for chunk in upload_file.chunks():
                    remote_file.write(chunk)
            return Response_200(data={'path': remote_file_path, 'name': upload_file.name})
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        finally:
            self._close_sftp(ssh_client, sftp_client)

    @action(detail=True, methods=['post'], url_path='files/upload/chunk')
    def webssh_file_upload_chunk(self, request, id=None):
        host = self.get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        upload_chunk = request.FILES.get('chunk')
        file_name = (request.data.get('filename') or '').strip()
        target_path = (request.data.get('path') or '.').strip()
        upload_id = _safe_upload_id(request.data.get('upload_id'))
        if upload_chunk is None:
            return Response_error_str('缺少上传分片', code=400)
        if not file_name:
            return Response_error_str('filename 不能为空', code=400)
        if '/' in file_name:
            return Response_error_str('filename 不能包含路径分隔符', code=400)
        if not upload_id:
            return Response_error_str('upload_id 非法', code=400)
        try:
            chunk_index = int(request.data.get('chunk_index', 0))
            total_chunks = int(request.data.get('total_chunks', 0))
        except (TypeError, ValueError):
            return Response_error_str('chunk_index/total_chunks 非法', code=400)
        if chunk_index < 0 or total_chunks <= 0 or chunk_index >= total_chunks:
            return Response_error_str('chunk_index/total_chunks 超出范围', code=400)

        ssh_client = None
        sftp_client = None
        try:
            ssh_client, sftp_client = self._connect_sftp(host)
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
                return Response_200(data={
                    'done': True,
                    'path': remote_target_path,
                    'name': file_name,
                    'upload_id': upload_id,
                    'uploaded_chunks': chunk_index + 1,
                    'total_chunks': total_chunks,
                })

            return Response_200(data={
                'done': False,
                'upload_id': upload_id,
                'uploaded_chunks': chunk_index + 1,
                'total_chunks': total_chunks,
            })
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        finally:
            self._close_sftp(ssh_client, sftp_client)

    @action(detail=True, methods=['post'], url_path='files/upload/cancel')
    def webssh_file_upload_cancel(self, request, id=None):
        host = self.get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        file_name = (request.data.get('filename') or '').strip()
        target_path = (request.data.get('path') or '.').strip()
        upload_id = _safe_upload_id(request.data.get('upload_id'))
        if not file_name:
            return Response_error_str('filename 不能为空', code=400)
        if '/' in file_name:
            return Response_error_str('filename 不能包含路径分隔符', code=400)
        if not upload_id:
            return Response_error_str('upload_id 非法', code=400)
        ssh_client = None
        sftp_client = None
        try:
            ssh_client, sftp_client = self._connect_sftp(host)
            normalized_dir = sftp_client.normalize(target_path)
            temp_name = f'.{file_name}.{upload_id}.part'
            remote_temp_path = posixpath.join(normalized_dir.rstrip('/'), temp_name) if normalized_dir != '/' else f'/{temp_name}'
            try:
                sftp_client.remove(remote_temp_path)
            except Exception:
                pass
            return Response_200(data={'upload_id': upload_id, 'canceled': True})
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        finally:
            self._close_sftp(ssh_client, sftp_client)

    @action(detail=True, methods=['get'], url_path='files/upload/status')
    def webssh_file_upload_status(self, request, id=None):
        host = self.get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        file_name = (request.query_params.get('filename') or '').strip()  # type: ignore[union-attr]
        target_path = (request.query_params.get('path') or '.').strip()  # type: ignore[union-attr]
        upload_id = _safe_upload_id(request.query_params.get('upload_id'))  # type: ignore[union-attr]
        try:
            chunk_size = int(request.query_params.get('chunk_size') or (8 * 1024 * 1024))  # type: ignore[union-attr]
        except (TypeError, ValueError):
            return Response_error_str('chunk_size 非法', code=400)
        if chunk_size <= 0:
            return Response_error_str('chunk_size 必须大于 0', code=400)
        if not file_name:
            return Response_error_str('filename 不能为空', code=400)
        if '/' in file_name:
            return Response_error_str('filename 不能包含路径分隔符', code=400)
        if not upload_id:
            return Response_error_str('upload_id 非法', code=400)

        ssh_client = None
        sftp_client = None
        try:
            ssh_client, sftp_client = self._connect_sftp(host)
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
            return Response_200(data={
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
            return Response_error_str(str(exc), code=400)
        finally:
            self._close_sftp(ssh_client, sftp_client)

    @action(detail=True, methods=['post'], url_path='files/rename')
    def webssh_file_rename(self, request, id=None):
        host = self.get_object()
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
        try:
            ssh_client, sftp_client = self._connect_sftp(host)
            normalized_old_path = sftp_client.normalize(remote_path)
            new_path = posixpath.join(posixpath.dirname(normalized_old_path), new_name)
            sftp_client.rename(normalized_old_path, new_path)
            return Response_200(data={'path': new_path, 'name': new_name})
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        finally:
            self._close_sftp(ssh_client, sftp_client)

    @action(detail=True, methods=['delete'], url_path='files/delete')
    def webssh_file_delete(self, request, id=None):
        host = self.get_object()
        guard_response = self._guard_webssh_file_access(request, host)
        if guard_response is not None:
            return guard_response
        remote_path = (request.data.get('path') or '').strip()
        recursive = bool(request.data.get('recursive'))
        if not remote_path:
            return Response_error_str('path 不能为空', code=400)
        ssh_client = None
        sftp_client = None
        try:
            ssh_client, sftp_client = self._connect_sftp(host)
            normalized_path = sftp_client.normalize(remote_path)
            target_stat = sftp_client.lstat(normalized_path)
            if stat.S_ISDIR(target_stat.st_mode):
                if not recursive:
                    return Response_error_str('目录删除需要 recursive=true', code=400)
                self._remove_remote_path(sftp_client, normalized_path)
            else:
                sftp_client.remove(normalized_path)
            return Response_200(data={'path': normalized_path})
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        finally:
            self._close_sftp(ssh_client, sftp_client)

    @action(detail=True, methods=['post'], url_path='files/create-dir')
    def webssh_file_create_dir(self, request, id=None):
        host = self.get_object()
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
        try:
            ssh_client, sftp_client = self._connect_sftp(host)
            normalized_dir = sftp_client.normalize(target_path)
            new_dir_path = posixpath.join(normalized_dir.rstrip('/'), name) if normalized_dir != '/' else f'/{name}'
            sftp_client.mkdir(new_dir_path)
            return Response_200(data={'path': new_dir_path, 'name': name})
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        finally:
            self._close_sftp(ssh_client, sftp_client)

    @action(detail=True, methods=['post'], url_path='files/create-file')
    def webssh_file_create_file(self, request, id=None):
        host = self.get_object()
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
        try:
            ssh_client, sftp_client = self._connect_sftp(host)
            normalized_dir = sftp_client.normalize(target_path)
            new_file_path = posixpath.join(normalized_dir.rstrip('/'), name) if normalized_dir != '/' else f'/{name}'
            with sftp_client.file(new_file_path, 'xb') as remote_file:
                remote_file.write(b'')
            return Response_200(data={'path': new_file_path, 'name': name})
        except Exception as exc:
            return Response_error_str(str(exc), code=400)
        finally:
            self._close_sftp(ssh_client, sftp_client)

    @action(detail=True, methods=['post'], url_path='collect-info')
    def collect_info(self, request, id=None):
        host = self.get_object()
        host_display_name = self._display_host_name(host)
        try:
            collect_host_info(host)
            return Response_200(data={
                'id': host.id, 
                'status': 'collected',
                'message': f'主机 {host_display_name} 采集成功'
            })
        except Exception as exc:
            error_msg = str(exc)
            # 返回失败结果而不是抛异常，这样前端能看到具体的错误信息
            return Response_200(data={
                'id': host.id,
                'status': 'failed',
                'error': error_msg,
                'message': f'主机 {host_display_name} 采集失败：{error_msg}'
            })

    @action(detail=False, methods=['post'], url_path='batch-collect-info')
    def batch_collect_info(self, request):
        ids = request.data.get('ids', [])
        results = []
        for host in Host.objects.filter(id__in=ids):
            try:
                collect_host_info(host)
                results.append({'id': host.id, 'status': 'collected'})  # type: ignore[attr-defined]
            except Exception as exc:
                results.append({'id': host.id, 'status': 'failed', 'error': str(exc)})  # type: ignore[attr-defined]
        return Response_200(data={'results': results})

    @action(detail=False, methods=['post'], url_path='collect-all')
    def collect_all(self, request):
        collect_all_hosts_info()
        return Response_200(data={'status': 'collect_all_started'})

    