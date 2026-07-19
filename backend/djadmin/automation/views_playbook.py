from __future__ import annotations

import os
import subprocess
import tempfile

from ansible.errors import AnsibleError, AnsibleParserError

from .view_helpers import *
from .local_runner import run_job_in_background

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

    @staticmethod
    def _validate_ansible_playbook_syntax_or_raise(content: str) -> None:
        content_text = str(content or '').strip()
        if not content_text:
            raise serializers.ValidationError('Playbook content cannot be empty')

        temp_path = ''
        try:
            with tempfile.NamedTemporaryFile(mode='w', suffix='.yml', delete=False, encoding='utf-8') as temp_file:
                temp_file.write(content_text)
                temp_file.flush()
                temp_path = temp_file.name

            env = os.environ.copy()
            env.setdefault('ANSIBLE_NOCOLOR', '1')
            result = subprocess.run(
                ['ansible-playbook', '--syntax-check', '-i', 'localhost,', temp_path],
                text=True,
                capture_output=True,
                check=False,
                env=env,
            )
            if result.returncode != 0:
                raw_message = '\n'.join(
                    part.strip() for part in [result.stderr, result.stdout] if part and part.strip()
                ).strip()
                message_text = raw_message or 'syntax check failed'
                raise serializers.ValidationError(f'Playbook Ansible syntax error: {message_text}')
        except serializers.ValidationError:
            raise
        except FileNotFoundError as exc:
            raise serializers.ValidationError('Playbook Ansible syntax error: ansible-playbook command not found') from exc
        except (AnsibleParserError, AnsibleError, Exception) as exc:
            error_text = str(exc).replace(temp_path, '<playbook>').strip() if temp_path else str(exc).strip()
            raise serializers.ValidationError(f'Playbook Ansible syntax error: {error_text}') from exc
        finally:
            if temp_path and os.path.exists(temp_path):
                os.unlink(temp_path)

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
            self._validate_ansible_playbook_syntax_or_raise(content)
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
            self._validate_ansible_playbook_syntax_or_raise(content)
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

        job = AutomationExecutionJob.objects.create(
            status=AutomationExecutionJob.Status.PENDING,
            trigger_type=AutomationExecutionJob.TriggerType.MANUAL,
            inventory_snapshot=inventory_snapshot,
            task_name_snapshot='',
            template_name_snapshot=template.name or '',
            template_content_snapshot=template.content or '',
            extra_vars=extra_vars,
            requested_user_id=user_info.get('user_id'),
            requested_username=user_info.get('username', ''),
            result_summary={'message': 'Job created and waiting for runner'},
            # 权限提升配置快照（直接运行模板使用默认值）
            become_enabled_snapshot=False,
            become_method_snapshot='sudo',
            become_user_snapshot='root',
        )

        try:
            # 非 Celery：改为本地后台线程执行，避免阻塞 API 响应。
            run_job_in_background(int(job.id))
        except Exception as exc:
            job.status = AutomationExecutionJob.Status.FAILED
            job.result_summary = {'message': f'Failed to start local runner: {str(exc)}'}
            job.save(update_fields=['status', 'result_summary'])
            return Response_error_str(f'Job execution failed: {str(exc)}', code=400)

        serializer = AutomationExecutionJobSerializer(job)
        return Response_200(data=serializer.data)


