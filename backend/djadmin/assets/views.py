import warnings
from asgiref.sync import async_to_sync

from cryptography.utils import CryptographyDeprecationWarning
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
from django.db.models import Prefetch
from .webssh_runtime import WebSSHRuntimeRegistry
from .webssh_host_mixin import WebSSHHostMixin

warnings.filterwarnings(
    'ignore',
    message='.*TripleDES has been moved to cryptography\\.hazmat\\.decrepit\\.ciphers\\.algorithms\\.TripleDES.*',
    category=CryptographyDeprecationWarning,
)

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

    def _force_close_webssh_sessions_for_credential(self, credential_id):
        # 仅影响“把该凭证作为默认凭证”的主机，避免误踢无关会话。
        host_ids = list(
            HostCredential.objects.filter(
                credential_id=credential_id,
                is_default=True,
            ).values_list('host_id', flat=True)
        )
        if not host_ids:
            return 0
        return async_to_sync(WebSSHRuntimeRegistry.close_active_sessions_for_hosts)(
            host_ids,
            message='SSH 凭证已变更，连接已关闭',
            close_code=4411,
        )

    @staticmethod
    def _get_credential_connection_signature(credential):
        # 连接签名用于判断“会话连接参数”是否发生变化。
        return (
            str(credential.username or ''),
            str(credential.password or ''),
            str(credential.private_key or ''),
            int(credential.port or 22),
            int(credential.auth_type or 0),
        )

    def update(self, request, *args, **kwargs):
        instance = self.get_object()
        # 先比较变更前后签名，只有连接关键字段变化才断开会话。
        previous_signature = self._get_credential_connection_signature(instance)
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        instance.refresh_from_db()
        current_signature = self._get_credential_connection_signature(instance)
        if previous_signature != current_signature:
            self._force_close_webssh_sessions_for_credential(instance.id)
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        previous_signature = self._get_credential_connection_signature(instance)
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        serializer.save()
        instance.refresh_from_db()
        current_signature = self._get_credential_connection_signature(instance)
        if previous_signature != current_signature:
            self._force_close_webssh_sessions_for_credential(instance.id)
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

    def _get_group_and_subgroups(self, group_id):
        """递归获取分组及其所有子分组 ID。"""
        group_ids = [int(group_id)]
        children = HostGroup.objects.filter(parent_id=group_id).values_list('id', flat=True)
        for child_id in children:
            group_ids.extend(self._get_group_and_subgroups(int(child_id)))
        return group_ids

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

        target_group_ids = self._get_group_and_subgroups(instance.id)
        target_group_id_set = set(target_group_ids)

        referenced = []
        for inventory in AutomationInventory.objects.only('id', 'name', 'selected_group_ids'):
            group_ids = [int(item) for item in (inventory.selected_group_ids or []) if str(item).isdigit()]
            if target_group_id_set.intersection(set(group_ids)):
                referenced.append(inventory)

        if referenced:
            names = '、'.join(item.name for item in referenced[:5])
            suffix = '' if len(referenced) <= 5 else f' 等{len(referenced)}个'
            return Response_error_str(
                f'该主机组已被 Inventory 引用，不能删除。受影响 Inventory: {names}{suffix}',
                code=400,
            )

        deleted_id = instance.id
        # 删除分组前先删除该分组树下所有主机，避免 Host.group=SET_NULL 导致主机残留。
        Host.objects.filter(group_id__in=target_group_ids).delete()
        HostGroup.objects.filter(id__in=target_group_ids).delete()
        return Response_200(data={"id": deleted_id})

    @action(detail=False, methods=['delete'], url_path='batch-delete')
    def batchDelete(self, request):
        ids = request.data.get('ids', [])
        if not ids:
            return Response_200(data=[])

        from automation.models import AutomationInventory

        normalized_ids = [int(item) for item in ids if str(item).isdigit()]
        delete_group_ids = set()
        for group_id in normalized_ids:
            delete_group_ids.update(self._get_group_and_subgroups(group_id))

        referenced_pairs = []
        for inventory in AutomationInventory.objects.only('id', 'name', 'selected_group_ids'):
            group_ids = [int(item) for item in (inventory.selected_group_ids or []) if str(item).isdigit()]
            hit_ids = sorted(set(group_ids).intersection(delete_group_ids))
            if hit_ids:
                referenced_pairs.append((inventory.name, hit_ids))

        if referenced_pairs:
            sample = '；'.join(f"{name} -> {','.join(str(i) for i in group_ids)}" for name, group_ids in referenced_pairs[:3])
            suffix = '' if len(referenced_pairs) <= 3 else f' 等{len(referenced_pairs)}个 Inventory'
            return Response_error_str(
                f'批量删除中包含被 Inventory 引用的主机组，操作已阻止：{sample}{suffix}',
                code=400,
            )

        # 批量删除时，统一删除目标分组树下主机与分组。
        Host.objects.filter(group_id__in=list(delete_group_ids)).delete()
        deleted_count, _ = HostGroup.objects.filter(id__in=list(delete_group_ids)).delete()
        return Response_200(data={"deleted_count": deleted_count})


class HostManage(WebSSHHostMixin, GenericViewSet,CreateModelMixin,DestroyModelMixin,UpdateModelMixin,RetrieveModelMixin,ListModelMixin):
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
        'webssh_file_upload_chunk': 'assets:hosts:update',
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
        # 主机连接签名：IP + SSH 端口 + 默认凭证。
        previous_signature = (
            str(instance.ip or ''),
            int(instance.port or 22),
            self._get_default_credential_id_for_host(instance),
        )
        serializer = self.get_serializer(instance, data=request.data, partial=False)
        serializer.is_valid(raise_exception=True)
        self.perform_update(serializer)
        instance.refresh_from_db()
        current_signature = (
            str(instance.ip or ''),
            int(instance.port or 22),
            self._get_default_credential_id_for_host(instance),
        )
        if previous_signature != current_signature:
            # 连接参数变化后，已有 SSH 通道不再可信，需强制重建。
            self._force_close_webssh_sessions_for_hosts([instance.id])
        return Response_200(data=serializer.data)

    def partial_update(self, request, *args, **kwargs):
        instance = self.get_object()
        previous_signature = (
            str(instance.ip or ''),
            int(instance.port or 22),
            self._get_default_credential_id_for_host(instance),
        )
        serializer = self.get_serializer(instance, data=request.data, partial=True)
        serializer.is_valid(raise_exception=True)
        self.perform_update(serializer)
        instance.refresh_from_db()
        current_signature = (
            str(instance.ip or ''),
            int(instance.port or 22),
            self._get_default_credential_id_for_host(instance),
        )
        if previous_signature != current_signature:
            self._force_close_webssh_sessions_for_hosts([instance.id])
        return Response_200(data=serializer.data)

    def destroy(self, request, *args, **kwargs):
        instance = self.get_object()
        deleted_id = instance.id
        self._force_close_webssh_sessions_for_hosts([deleted_id])
        self.perform_destroy(instance)
        return Response_200(data={"id": deleted_id})

    @action(detail=False, methods=['delete'], url_path='batch-delete')
    def batchDelete(self, request):
        ids = request.data.get('ids', [])
        if not ids:
            return Response_200(data=[])
        normalized_ids = []
        for item in ids:
            text = str(item).strip()
            if not text.isdigit():
                continue
            value = int(text)
            if value > 0:
                normalized_ids.append(value)
        self._force_close_webssh_sessions_for_hosts(normalized_ids)
        host_queryset = Host.objects.filter(id__in=normalized_ids)
        deleted_count = host_queryset.count()
        host_queryset.delete()
        return Response_200(data={"deleted_count": deleted_count})

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

    