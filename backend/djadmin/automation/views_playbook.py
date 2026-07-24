from __future__ import annotations

import os
import subprocess
import tempfile

from ansible.errors import AnsibleError, AnsibleParserError
from django.utils import timezone

from .view_helpers import *
from .view_helpers import _is_playbook_template_bound_to_software_package
from .models import TemplateCategory
from .agent_grpc_runner import execute_job_via_agent_grpc

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

    def get_queryset(self):
        # 可选 category 过滤：前端“模板”列表页默认只看“通用”分类，
        # 软件包安装/卸载专用模板需要显式切换筛选才会出现，避免和普通运维 playbook 混在一起。
        queryset = super().get_queryset()
        category = (self.request.query_params.get('category') or '').strip()  # type: ignore[union-attr]
        if category:
            queryset = queryset.filter(category=category)
        return queryset

    def perform_update(self, serializer):
        # 分类降级保护：模板正被监控软件仓库的 install_playbook_template/uninstall_playbook_template
        # 直接引用时，禁止把 category 从 software_package 改回 general，防止误操作后在监控软件仓库里找不到对应模板。
        instance = serializer.instance
        new_category = serializer.validated_data.get('category', instance.category)
        if instance.category == TemplateCategory.SOFTWARE_PACKAGE and new_category != TemplateCategory.SOFTWARE_PACKAGE:
            if _is_playbook_template_bound_to_software_package(instance.id):
                raise serializers.ValidationError({
                    'category': '该模板正被监控软件仓库的安装/卸载引用，无法改为“通用”分类',
                })
        serializer.save()

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

        # 直接运行 Playbook 模板不经过 AutomationTask，没有任务上带的 run_as_user/run_as_group 可用，
        # 必须由调用方显式传入执行身份，禁止静默以 dj-agent 进程自身身份（root）执行。
        run_as_user = str(request.data.get('run_as_user') or '').strip()
        if run_as_user == '':
            return Response_error_str('run_as_user is required to run a playbook template directly', code=400)
        run_as_group = str(request.data.get('run_as_group') or '').strip()
        work_directory = str(request.data.get('work_directory') or '').strip() or '/tmp'

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        group_ids = group_ids_raw if isinstance(group_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        group_ids = [int(item) for item in group_ids if str(item).isdigit()]

        if len(host_ids) == 0 and len(group_ids) == 0:
            return Response_error_str('No target hosts selected', code=400)

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

        started_at = timezone.now()
        job = AutomationExecutionJob.objects.create(
            status=AutomationExecutionJob.Status.RUNNING,
            trigger_type=AutomationExecutionJob.TriggerType.MANUAL,
            inventory_snapshot=inventory_snapshot,
            task_name_snapshot='',
            template_name_snapshot=template.name or '',
            template_content_snapshot=template.content or '',
            extra_vars=extra_vars,
            requested_user_id=user_info.get('user_id'),
            requested_username=user_info.get('username', ''),
            result_summary={'message': 'Job created and executing via agent grpc'},
            start_time=started_at,
            # 直接运行 Playbook 模板不经过 AutomationTask，需要请求方显式传入执行身份（已在前面校验 run_as_user 非空）。
            run_as_user_snapshot=run_as_user,
            run_as_group_snapshot=run_as_group,
            work_directory_snapshot=work_directory,
        )

        try:
            hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
            success, summary, _ = execute_job_via_agent_grpc(
                automation_execution_job_id=int(job.id),
                automation_task_id=0,
                template_content=job.template_content_snapshot or '',
                template_type='playbook',
                hosts=hosts,
                shell_parameters='',
                shell_env_vars={},
                extra_vars=extra_vars,
                run_as_user=run_as_user,
                run_as_group=run_as_group,
                work_directory=work_directory,
                timeout_seconds=600,
            )
            finished_at = timezone.now()
            final_status = AutomationExecutionJob.Status.SUCCESS if success else AutomationExecutionJob.Status.FAILED
            job.status = final_status
            job.end_time = finished_at
            job.duration_seconds = (finished_at - started_at).total_seconds()
            job.result_summary = summary
            job.save(update_fields=['status', 'result_summary', 'end_time', 'duration_seconds'])
        except Exception as exc:
            finished_at = timezone.now()
            job.status = AutomationExecutionJob.Status.FAILED
            job.end_time = finished_at
            job.duration_seconds = (finished_at - started_at).total_seconds()
            job.result_summary = {'message': f'Job execution failed: {str(exc)}'}
            job.save(update_fields=['status', 'result_summary', 'end_time', 'duration_seconds'])
            return Response_error_str(f'Job execution failed: {str(exc)}', code=400)

        serializer = AutomationExecutionJobSerializer(job)
        return Response_200(data=serializer.data)


