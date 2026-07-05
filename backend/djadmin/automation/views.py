import os
import re
import fnmatch
from urllib.parse import quote

from django.http import HttpResponse
from django.utils.dateparse import parse_datetime
from django.db.models import Q
from django_filters.rest_framework import DjangoFilterBackend
from rest_framework.decorators import action
from rest_framework.filters import OrderingFilter, SearchFilter
from rest_framework.mixins import CreateModelMixin, DestroyModelMixin, ListModelMixin, RetrieveModelMixin, UpdateModelMixin
from rest_framework import serializers
from rest_framework.viewsets import GenericViewSet

from djadmin.utils import CustomPagination, Response_200, Response_error_str
from menu.permisssion import CustomMenuPermission
from user.utils import getCurrentUser
from assets.models import Host, HostGroup

from .models import PlaybookTemplate, AutomationTask, AutomationInventory, AnsibleExecutionJob, AnsibleExecutionTarget
from .serializer import (
    PlaybookTemplateSerializer,
    AutomationTaskSerializer,
    AutomationInventorySerializer,
    AnsibleExecutionJobSerializer,
    AnsibleExecutionTargetSerializer,
    validate_playbook_content_or_raise,
)
from .executor import build_inventory_snapshot
from .tasks import execute_ansible_job_task


def _parse_limit_tokens(limit_text: str) -> tuple[list[str], list[str]]:
    tokens = [token.strip() for token in re.split(r'[\s,]+', str(limit_text or '').strip()) if token.strip()]
    include_tokens = []
    exclude_tokens = []
    for token in tokens:
        if token.startswith('!') and len(token) > 1:
            exclude_tokens.append(token[1:])
        else:
            include_tokens.append(token)
    return include_tokens, exclude_tokens


def _match_limit_token(host_item: dict, token: str) -> bool:
    pattern = str(token or '').strip().lower()
    if not pattern:
        return False

    host_id_text = str(host_item.get('host_id') or '')
    host_name = str(host_item.get('host_name') or '').lower()
    host_ip = str(host_item.get('host_ip') or '').lower()
    group_name = str(host_item.get('group_name') or '').lower()
    group_path = str(host_item.get('group_path') or '').lower()
    if fnmatch.fnmatch(host_id_text, pattern):
        return True
    if fnmatch.fnmatch(host_name, pattern):
        return True
    if fnmatch.fnmatch(host_ip, pattern):
        return True
    if fnmatch.fnmatch(group_name, pattern):
        return True
    if fnmatch.fnmatch(group_path, pattern):
        return True
    return False


def _sort_inventory_hosts(hosts: list[dict]) -> list[dict]:
    normalized_hosts = [item for item in hosts if isinstance(item, dict)]
    return sorted(
        normalized_hosts,
        key=lambda item: (
            str(item.get('group_path') or item.get('group_name') or '').lower(),
            str(item.get('host_name') or '').lower(),
            str(item.get('host_ip') or ''),
            int(item.get('host_id') or 0),
        ),
    )


def _build_limit_matched_hosts_preview(inventory_snapshot: dict, preview_size: int | None = None) -> list[dict]:
    if not isinstance(inventory_snapshot, dict):
        return []
    hosts = inventory_snapshot.get('hosts', [])
    if not isinstance(hosts, list):
        return []

    sorted_hosts = _sort_inventory_hosts(hosts)
    if preview_size is None:
        target_hosts = sorted_hosts
    else:
        target_hosts = sorted_hosts[:preview_size]

    preview_items = []
    for item in target_hosts:
        preview_items.append({
            'host_id': item.get('host_id'),
            'host_name': item.get('host_name') or '',
            'host_ip': item.get('host_ip') or '',
            'group_name': item.get('group_name') or '',
            'group_path': item.get('group_path') or '',
        })
    return preview_items


def _apply_limit_to_inventory_snapshot(inventory_snapshot: dict, limit_text: str) -> dict:
    if not isinstance(inventory_snapshot, dict):
        return {'hosts': []}

    normalized_limit = str(limit_text or '').strip()
    hosts = inventory_snapshot.get('hosts', [])
    if not isinstance(hosts, list):
        hosts = []

    if not normalized_limit:
        next_snapshot = {**inventory_snapshot}
        next_snapshot['hosts'] = _sort_inventory_hosts(hosts)
        return next_snapshot

    include_tokens, exclude_tokens = _parse_limit_tokens(normalized_limit)

    filtered_hosts = []
    for host_item in hosts:
        if not isinstance(host_item, dict):
            continue

        include_ok = True
        if include_tokens:
            include_ok = any(_match_limit_token(host_item, token) for token in include_tokens)

        exclude_hit = any(_match_limit_token(host_item, token) for token in exclude_tokens)
        if include_ok and not exclude_hit:
            filtered_hosts.append(host_item)

    next_snapshot = {**inventory_snapshot}
    next_snapshot['hosts'] = _sort_inventory_hosts(filtered_hosts)
    next_snapshot['limit'] = normalized_limit
    return next_snapshot


class PlaybookTemplateManage(GenericViewSet, CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin):
    queryset = PlaybookTemplate.objects.all()
    serializer_class = PlaybookTemplateSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'description', 'remark']
    ordering_fields = ['name', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:playbooks:view',
        'retrieve': 'automation:playbooks:view',
        'validate_content': 'automation:playbooks:view',
        'create': 'automation:playbooks:create',
        'destroy': 'automation:playbooks:delete',
        'partial_update': 'automation:playbooks:update',
        'perform_update': 'automation:playbooks:update',
        'upload_file': 'automation:playbooks:update',
        'download_file': 'automation:playbooks:view',
        'run_template': 'automation:jobs:create',
        'host_options': 'automation:jobs:create',
        'group_tree': 'automation:jobs:create',
    }

    @staticmethod
    def _build_download_filename(template: PlaybookTemplate) -> str:
        raw_name = template.name or f'playbook-{template.id}'
        sanitized = re.sub(r'[^A-Za-z0-9._-]+', '-', raw_name).strip('-')
        return f'{sanitized or "playbook-template"}.yml'

    @staticmethod
    def _validation_error_to_text(exc: serializers.ValidationError) -> str:
        detail = exc.detail
        if isinstance(detail, list):
            return str(detail[0]) if detail else 'Invalid playbook content'
        if isinstance(detail, dict):
            first_value = next(iter(detail.values()), 'Invalid playbook content')
            if isinstance(first_value, list):
                return str(first_value[0]) if first_value else 'Invalid playbook content'
            return str(first_value)
        return str(detail)

    def _build_group_tree(self, groups_data):
        nodes = {}
        roots = []

        for item in groups_data:
            node = {
                'id': item['id'],
                'name': item['name'],
                'parent': item['parent_id'],
                'children': [],
            }
            nodes[item['id']] = node

        for node in nodes.values():
            parent_id = node.get('parent')
            if parent_id and parent_id in nodes:
                nodes[parent_id]['children'].append(node)
            else:
                roots.append(node)

        return roots

    @action(detail=False, methods=['post'], url_path='validate')
    def validate_content(self, request):
        content = request.data.get('content', '')
        try:
            validate_playbook_content_or_raise(content)
        except serializers.ValidationError as exc:
            return Response_error_str(self._validation_error_to_text(exc), code=400)
        return Response_200(data={'valid': True})

    @action(detail=False, methods=['get'], url_path='host-options')
    def host_options(self, request):
        keyword = (request.query_params.get('search') or '').strip()  # type: ignore[union-attr]
        # GenericIPAddressField does not store empty strings reliably on MySQL,
        # and exclude(ip='') may be translated into an invalid condition.
        queryset = Host.objects.filter(ip__isnull=False).select_related('system').order_by('id')

        if keyword:
            queryset = queryset.filter(
                Q(instance_name__icontains=keyword)
                | Q(system__hostname__icontains=keyword)
                | Q(ip__icontains=keyword)
            )

        page = self.paginate_queryset(queryset)
        records = page if page is not None else queryset[:30]
        data = [
            {
                'id': item.id,
                'instance_name': item.instance_name,
                'hostname': item.system.hostname if getattr(item, 'system', None) else None,
                'ip': item.ip,
                'group_id': item.group_id,
            }
            for item in records
        ]

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

        return Response_200(data={'count': len(data), 'results': data})

    @action(detail=False, methods=['get'], url_path='group-tree')
    def group_tree(self, request):
        groups = list(HostGroup.objects.all().order_by('id').values('id', 'name', 'parent_id'))
        return Response_200(data=self._build_group_tree(groups))

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
        return Response_200(data={'id': deleted_id})

    @action(detail=True, methods=['post'], url_path='upload')
    def upload_file(self, request, id=None):
        template = self.get_object()
        uploaded_file = request.FILES.get('file')
        if uploaded_file is None:
            return Response_error_str('Please upload a YAML file', code=400)

        suffix = os.path.splitext(uploaded_file.name or '')[1].lower()
        if suffix not in {'.yml', '.yaml'}:
            return Response_error_str('Only .yml or .yaml files are supported', code=400)

        try:
            content = uploaded_file.read().decode('utf-8')
        except UnicodeDecodeError:
            return Response_error_str('Template file must be UTF-8 encoded', code=400)

        if not content.strip():
            return Response_error_str('Template file is empty', code=400)

        try:
            validate_playbook_content_or_raise(content)
        except serializers.ValidationError as exc:
            return Response_error_str(self._validation_error_to_text(exc), code=400)

        template.content = content
        template.save(update_fields=['content', 'update_time'])

        serializer = self.get_serializer(template)
        return Response_200(data=serializer.data)

    @action(detail=True, methods=['get'], url_path='download')
    def download_file(self, request, id=None):
        template = self.get_object()
        filename = self._build_download_filename(template)
        response = HttpResponse(template.content or '', content_type='text/yaml; charset=utf-8')
        response['Content-Disposition'] = f"attachment; filename*=UTF-8''{quote(filename)}"
        return response

    @action(detail=True, methods=['post'], url_path='run')
    def run_template(self, request, id=None):
        template = self.get_object()

        user_info = getCurrentUser(request)
        host_ids_raw = request.data.get('host_ids', [])
        group_ids_raw = request.data.get('group_ids', [])
        inventory_snapshot = request.data.get('inventory_snapshot', {})
        extra_vars = request.data.get('extra_vars', {})

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        group_ids = group_ids_raw if isinstance(group_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        group_ids = [int(item) for item in group_ids if str(item).isdigit()]

        if len(host_ids) == 0 and len(group_ids) == 0:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_empty',
                'message': f'Inventory [{inventory.name or "-"}] 未选择主机组，当前无可执行主机',
                'resolved_host_count': 0,
                'effective_limit': limit_text,
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
            })

        if host_ids or group_ids:
            inventory_snapshot = build_inventory_snapshot(host_ids=host_ids, group_ids=group_ids)
        elif isinstance(inventory_snapshot, dict) and inventory_snapshot.get('hosts'):
            pass
        elif isinstance(inventory_snapshot, dict):
            inventory_snapshot = build_inventory_snapshot(host_ids=[], group_ids=[])
        else:
            return Response_error_str('inventory_snapshot must be an object', code=400)

        if isinstance(inventory_snapshot, dict):
            hosts = inventory_snapshot.get('hosts', [])
            if not isinstance(hosts, list):
                return Response_error_str('inventory_snapshot.hosts must be a list', code=400)
            if len(hosts) == 0:
                return Response_error_str('No target hosts selected', code=400)

            # Filter invalid or deleted hosts before creating job.
            valid_host_ids = [item.get('host_id') for item in hosts if isinstance(item, dict)]
            valid_host_ids = [
                int(item) for item in valid_host_ids
                if item is not None and str(item).isdigit()
            ]
            existing_ids = set(Host.objects.filter(id__in=valid_host_ids).values_list('id', flat=True))
            inventory_snapshot['hosts'] = [
                item for item in hosts
                if isinstance(item, dict)
                and item.get('host_id') is not None
                and str(item.get('host_id')).isdigit()
                and int(item.get('host_id')) in existing_ids
            ]
            if len(inventory_snapshot['hosts']) == 0:
                return Response_error_str('No valid hosts found in selection', code=400)

        if not isinstance(extra_vars, dict):
            return Response_error_str('extra_vars must be an object', code=400)

        job = AnsibleExecutionJob.objects.create(
            template=template,
            status=AnsibleExecutionJob.Status.PENDING,
            trigger_type=AnsibleExecutionJob.TriggerType.MANUAL,
            inventory_snapshot=inventory_snapshot,
            task_name_snapshot='',
            template_name_snapshot=template.name or '',
            template_content_snapshot=template.content or '',
            extra_vars=extra_vars,
            requested_user_id=user_info.get('user_id'),
            requested_username=user_info.get('username', ''),
            result_summary={'message': 'Job created and waiting for runner'},
        )

        try:
            execute_ansible_job_task.delay(job.id)
        except Exception as exc:
            job.status = AnsibleExecutionJob.Status.FAILED
            job.result_summary = {'message': f'Failed to enqueue job: {str(exc)}'}
            job.save(update_fields=['status', 'result_summary'])
            return Response_error_str(f'Job enqueue failed: {str(exc)}', code=400)

        serializer = AnsibleExecutionJobSerializer(job)
        return Response_200(data=serializer.data)


class AutomationTaskManage(GenericViewSet, CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin):
    queryset = AutomationTask.objects.select_related('template', 'inventory').all()
    serializer_class = AutomationTaskSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'code', 'template__name', 'inventory__name', 'remark']
    ordering_fields = ['name', 'code', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:tasks:view',
        'retrieve': 'automation:tasks:view',
        'create': 'automation:tasks:create',
        'destroy': 'automation:tasks:delete',
        'partial_update': 'automation:tasks:update',
        'perform_update': 'automation:tasks:update',
        'precheck': 'automation:jobs:create',
        'run_now': 'automation:jobs:create',
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
        return Response_200(data={'id': deleted_id})

    @action(detail=True, methods=['post'], url_path='precheck')
    def precheck(self, request, id=None):
        task = self.get_object()

        if not task.enabled:
            return Response_200(data={
                'ok': False,
                'status': 'task_disabled',
                'message': '任务已禁用，无法执行',
                'resolved_host_count': 0,
            })

        if not task.template:
            return Response_200(data={
                'ok': False,
                'status': 'template_missing',
                'message': '任务模板不存在',
                'resolved_host_count': 0,
            })

        if task.inventory_id and task.inventory is not None and not task.inventory.enabled:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_disabled',
                'message': 'Inventory 已禁用，无法执行',
                'resolved_host_count': 0,
            })

        default_host_ids = task.selected_host_ids
        default_group_ids = task.selected_group_ids
        inventory_name = ''
        if task.inventory_id and task.inventory is not None:
            default_host_ids = task.inventory.selected_host_ids
            default_group_ids = task.inventory.selected_group_ids
            inventory_name = task.inventory.name or ''

        limit_text = str(request.data.get('limit', task.default_limit or '')).strip()

        host_ids_raw = request.data.get('host_ids', default_host_ids)
        group_ids_raw = request.data.get('group_ids', default_group_ids)

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        group_ids = group_ids_raw if isinstance(group_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        group_ids = [int(item) for item in group_ids if str(item).isdigit()]

        if task.inventory_id and task.inventory is not None and len(host_ids) == 0 and len(group_ids) == 0:
            inventory_name = task.inventory.name or '-'
            return Response_200(data={
                'ok': False,
                'status': 'inventory_empty',
                'message': f'Inventory [{inventory_name}] 未选择主机组，当前无可执行主机',
                'resolved_host_count': 0,
            })

        existing_group_ids = set(HostGroup.objects.filter(id__in=group_ids).values_list('id', flat=True))
        missing_group_ids = sorted(set(group_ids) - existing_group_ids)
        if missing_group_ids:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_invalid',
                'message': f'执行范围包含已删除主机组: {", ".join(str(item) for item in missing_group_ids)}',
                'resolved_host_count': 0,
                'missing_group_ids': missing_group_ids,
            })

        inventory_snapshot = build_inventory_snapshot(host_ids=host_ids, group_ids=group_ids)
        inventory_snapshot = _apply_limit_to_inventory_snapshot(inventory_snapshot, limit_text)
        hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
        resolved_host_count = len(hosts) if isinstance(hosts, list) else 0
        if resolved_host_count == 0:
            if inventory_name:
                message = f'Inventory [{inventory_name}] 当前无可用主机，请检查主机组是否被删除或范围配置是否正确'
            else:
                message = '当前任务无可用主机，请检查执行范围配置'
            return Response_200(data={
                'ok': False,
                'status': 'inventory_empty',
                'message': message,
                'resolved_host_count': 0,
            })

        return Response_200(data={
            'ok': True,
            'status': 'ok',
            'message': f'预检通过，可执行主机 {resolved_host_count} 台',
            'resolved_host_count': resolved_host_count,
            'effective_limit': limit_text,
            'matched_hosts_preview': _build_limit_matched_hosts_preview(inventory_snapshot),
            'matched_hosts_preview_total': resolved_host_count,
        })

    @action(detail=True, methods=['post'], url_path='run_now')
    def run_now(self, request, id=None):
        task = self.get_object()
        if not task.enabled:
            return Response_error_str('Task is disabled', code=400)
        if not task.template:
            return Response_error_str('Template is missing', code=400)
        if task.inventory_id and task.inventory is not None and not task.inventory.enabled:
            return Response_error_str('Inventory is disabled', code=400)

        user_info = getCurrentUser(request)
        default_host_ids = task.selected_host_ids
        default_group_ids = task.selected_group_ids
        if task.inventory_id and task.inventory is not None:
            default_host_ids = task.inventory.selected_host_ids
            default_group_ids = task.inventory.selected_group_ids

        limit_text = str(request.data.get('limit', task.default_limit or '')).strip()

        host_ids_raw = request.data.get('host_ids', default_host_ids)
        group_ids_raw = request.data.get('group_ids', default_group_ids)
        extra_vars_raw = request.data.get('extra_vars', task.env_vars)

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        group_ids = group_ids_raw if isinstance(group_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        group_ids = [int(item) for item in group_ids if str(item).isdigit()]
        extra_vars = extra_vars_raw if isinstance(extra_vars_raw, dict) else {}

        if task.inventory_id and task.inventory is not None and len(host_ids) == 0 and len(group_ids) == 0:
            inventory_name = task.inventory.name or '-'
            return Response_error_str(
                f'Inventory [{inventory_name}] 未选择主机组，当前无可执行主机',
                code=400,
            )

        inventory_snapshot = build_inventory_snapshot(host_ids=host_ids, group_ids=group_ids)
        inventory_snapshot = _apply_limit_to_inventory_snapshot(inventory_snapshot, limit_text)
        hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
        if not isinstance(hosts, list) or len(hosts) == 0:
            inventory_name = task.inventory.name if task.inventory_id and task.inventory else ''
            if inventory_name:
                return Response_error_str(
                    f'Inventory [{inventory_name}] 当前无可用主机，请检查主机组是否被删除或范围配置是否正确',
                    code=400,
                )
            return Response_error_str('当前任务无可用主机，请检查执行范围配置', code=400)

        job = AnsibleExecutionJob.objects.create(
            template=task.template,
            task=task,
            status=AnsibleExecutionJob.Status.PENDING,
            trigger_type=AnsibleExecutionJob.TriggerType.MANUAL,
            inventory_snapshot=inventory_snapshot,
            task_name_snapshot=task.name or '',
            template_name_snapshot=task.template.name or '',
            template_content_snapshot=task.template.content or '',
            extra_vars=extra_vars,
            limit=limit_text,
            requested_user_id=user_info.get('user_id'),
            requested_username=user_info.get('username', ''),
            result_summary={'message': 'Job created and waiting for runner'},
        )

        try:
            execute_ansible_job_task.delay(job.id)
        except Exception as exc:
            job.status = AnsibleExecutionJob.Status.FAILED
            job.result_summary = {'message': f'Failed to enqueue job: {str(exc)}'}
            job.save(update_fields=['status', 'result_summary'])
            return Response_error_str(f'Job enqueue failed: {str(exc)}', code=400)

        serializer = AnsibleExecutionJobSerializer(job)
        return Response_200(data=serializer.data)


class AutomationInventoryManage(GenericViewSet, CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin):
    queryset = AutomationInventory.objects.all()
    serializer_class = AutomationInventorySerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'remark']
    ordering_fields = ['name', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:inventory:view',
        'retrieve': 'automation:inventory:view',
        'create': 'automation:inventory:create',
        'destroy': 'automation:inventory:delete',
        'partial_update': 'automation:inventory:update',
        'perform_update': 'automation:inventory:update',
        'precheck_limit': 'automation:inventory:view',
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
        return Response_200(data={'id': deleted_id})

    @action(detail=True, methods=['post'], url_path='precheck-limit')
    def precheck_limit(self, request, id=None):
        inventory = self.get_object()

        if not inventory.enabled:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_disabled',
                'message': 'Inventory 已禁用，无法执行预检',
                'resolved_host_count': 0,
                'effective_limit': '',
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
            })

        limit_text = str(request.data.get('limit', '')).strip()
        host_ids_raw = request.data.get('host_ids', inventory.selected_host_ids)
        group_ids_raw = request.data.get('group_ids', inventory.selected_group_ids)

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        group_ids = group_ids_raw if isinstance(group_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        group_ids = [int(item) for item in group_ids if str(item).isdigit()]

        existing_group_ids = set(HostGroup.objects.filter(id__in=group_ids).values_list('id', flat=True))
        missing_group_ids = sorted(set(group_ids) - existing_group_ids)
        if missing_group_ids:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_invalid',
                'message': f'执行范围包含已删除主机组: {", ".join(str(item) for item in missing_group_ids)}',
                'resolved_host_count': 0,
                'effective_limit': limit_text,
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
                'missing_group_ids': missing_group_ids,
            })

        inventory_snapshot = build_inventory_snapshot(host_ids=host_ids, group_ids=group_ids)
        inventory_snapshot = _apply_limit_to_inventory_snapshot(inventory_snapshot, limit_text)
        hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
        resolved_host_count = len(hosts) if isinstance(hosts, list) else 0

        if resolved_host_count == 0:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_empty',
                'message': f'Inventory [{inventory.name or "-"}] 当前无匹配主机',
                'resolved_host_count': 0,
                'effective_limit': limit_text,
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
            })

        return Response_200(data={
            'ok': True,
            'status': 'ok',
            'message': f'预检通过，可匹配主机 {resolved_host_count} 台',
            'resolved_host_count': resolved_host_count,
            'effective_limit': limit_text,
            'matched_hosts_preview': _build_limit_matched_hosts_preview(inventory_snapshot),
            'matched_hosts_preview_total': resolved_host_count,
        })


class AnsibleExecutionJobManage(GenericViewSet, RetrieveModelMixin, ListModelMixin):
    queryset = AnsibleExecutionJob.objects.select_related('template', 'task').all()
    serializer_class = AnsibleExecutionJobSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['job_id', 'requested_username', 'template__name', 'task__name', 'remark']
    ordering_fields = ['create_time', 'update_time', 'start_time', 'end_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:jobs:view',
        'retrieve': 'automation:jobs:view',
        'cancel': 'automation:jobs:cancel',
        'log': 'automation:jobs:view',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        status_value = self.request.query_params.get('status')  # type: ignore[union-attr]
        template_id = self.request.query_params.get('template_id')  # type: ignore[union-attr]
        task_id = self.request.query_params.get('task_id')  # type: ignore[union-attr]
        keyword = self.request.query_params.get('keyword')  # type: ignore[union-attr]
        output_keyword = self.request.query_params.get('output_keyword')  # type: ignore[union-attr]
        start_time_from = self.request.query_params.get('start_time_from')  # type: ignore[union-attr]
        start_time_to = self.request.query_params.get('start_time_to')  # type: ignore[union-attr]

        if status_value:
            queryset = queryset.filter(status=status_value)
        if template_id:
            queryset = queryset.filter(template_id=template_id)
        if task_id:
            queryset = queryset.filter(task_id=task_id)
        if keyword:
            condition = (
                Q(job_id__icontains=keyword) |
                Q(requested_username__icontains=keyword) |
                Q(template__name__icontains=keyword) |
                Q(task__name__icontains=keyword)
            )
            if str(keyword).isdigit():
                condition = condition | Q(id=int(keyword))
            queryset = queryset.filter(condition)

        if output_keyword:
            queryset = queryset.filter(job_output__icontains=output_keyword)

        if start_time_from:
            parsed_from = parse_datetime(start_time_from)
            if parsed_from is not None:
                queryset = queryset.filter(start_time__gte=parsed_from)
        if start_time_to:
            parsed_to = parse_datetime(start_time_to)
            if parsed_to is not None:
                queryset = queryset.filter(start_time__lte=parsed_to)

        return queryset

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

    @action(detail=True, methods=['get'])
    def log(self, request, id=None):
        job = self.get_object()
        return Response_200(data={
            'job_id': job.id,
            'status': job.status,
            'job_output': job.job_output or '',
        })

    @action(detail=True, methods=['post'])
    def cancel(self, request, id=None):
        job = self.get_object()
        if job.status in (AnsibleExecutionJob.Status.SUCCESS, AnsibleExecutionJob.Status.FAILED, AnsibleExecutionJob.Status.CANCELLED):
            return Response_error_str('Job is already finished', code=400)

        now = timezone.now()
        if not job.start_time:
            job.start_time = now
        job.end_time = now
        if job.start_time:
            job.duration_seconds = (job.end_time - job.start_time).total_seconds()
        job.status = AnsibleExecutionJob.Status.CANCELLED
        job.result_summary = {'message': 'Cancelled by user'}
        job.save(update_fields=['status', 'start_time', 'end_time', 'duration_seconds', 'result_summary'])

        AnsibleExecutionTarget.objects.filter(job=job, status__in=[
            AnsibleExecutionTarget.Status.PENDING,
            AnsibleExecutionTarget.Status.RUNNING,
        ]).update(
            status=AnsibleExecutionTarget.Status.SKIPPED,
            end_time=now,
            stderr='Cancelled by user',
        )

        return Response_200(data=AnsibleExecutionJobSerializer(job).data)


class AnsibleExecutionTargetManage(GenericViewSet, RetrieveModelMixin, ListModelMixin):
    queryset = AnsibleExecutionTarget.objects.select_related('job', 'host').all()
    serializer_class = AnsibleExecutionTargetSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['host_name', 'host_ip', 'stdout', 'stderr', 'remark']
    ordering_fields = ['create_time', 'update_time', 'start_time', 'end_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:targets:view',
        'retrieve': 'automation:targets:view',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        job_id = self.request.query_params.get('job_id')  # type: ignore[union-attr]
        status_value = self.request.query_params.get('status')  # type: ignore[union-attr]
        if job_id:
            queryset = queryset.filter(job_id=job_id)
        if status_value:
            queryset = queryset.filter(status=status_value)
        return queryset

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
