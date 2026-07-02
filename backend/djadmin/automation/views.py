import threading

from django.utils import timezone
from django.db.models import Q
from django_filters.rest_framework import DjangoFilterBackend
from rest_framework.decorators import action
from rest_framework.filters import OrderingFilter, SearchFilter
from rest_framework.mixins import CreateModelMixin, DestroyModelMixin, ListModelMixin, RetrieveModelMixin, UpdateModelMixin
from rest_framework.viewsets import GenericViewSet

from djadmin.utils import CustomPagination, Response_200, Response_error_str
from menu.permisssion import CustomMenuPermission
from user.utils import getCurrentUser
from assets.models import Host, HostGroup

from .models import PlaybookTemplate, AnsibleExecutionJob, AnsibleExecutionTarget
from .serializer import PlaybookTemplateSerializer, AnsibleExecutionJobSerializer, AnsibleExecutionTargetSerializer
from .executor import build_inventory_snapshot, execute_ansible_job


class PlaybookTemplateManage(GenericViewSet, CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin):
    queryset = PlaybookTemplate.objects.all()
    serializer_class = PlaybookTemplateSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'code', 'description', 'remark']
    ordering_fields = ['name', 'code', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:playbooks:view',
        'retrieve': 'automation:playbooks:view',
        'create': 'automation:playbooks:create',
        'destroy': 'automation:playbooks:delete',
        'partial_update': 'automation:playbooks:update',
        'perform_update': 'automation:playbooks:update',
        'run_template': 'automation:jobs:create',
        'host_options': 'automation:jobs:create',
        'group_tree': 'automation:jobs:create',
    }

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

    @action(detail=True, methods=['post'], url_path='run')
    def run_template(self, request, id=None):
        template = self.get_object()
        if not template.enabled:
            return Response_error_str('Playbook template is disabled', code=400)

        user_info = getCurrentUser(request)
        host_ids_raw = request.data.get('host_ids', [])
        group_ids_raw = request.data.get('group_ids', [])
        inventory_snapshot = request.data.get('inventory_snapshot', {})
        extra_vars = request.data.get('extra_vars', {})

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        group_ids = group_ids_raw if isinstance(group_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        group_ids = [int(item) for item in group_ids if str(item).isdigit()]

        if host_ids or group_ids:
            inventory_snapshot = build_inventory_snapshot(host_ids=host_ids, group_ids=group_ids)
        elif not isinstance(inventory_snapshot, dict):
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
            extra_vars=extra_vars,
            requested_user_id=user_info.get('user_id'),
            requested_username=user_info.get('username', ''),
            result_summary={'message': 'Job created and waiting for runner'},
        )

        thread = threading.Thread(
            target=execute_ansible_job,
            args=(job.id,),
            daemon=True,
        )
        thread.start()

        serializer = AnsibleExecutionJobSerializer(job)
        return Response_200(data=serializer.data)


class AnsibleExecutionJobManage(GenericViewSet, RetrieveModelMixin, ListModelMixin):
    queryset = AnsibleExecutionJob.objects.select_related('template').all()
    serializer_class = AnsibleExecutionJobSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['job_id', 'requested_username', 'template__name', 'remark']
    ordering_fields = ['create_time', 'update_time', 'start_time', 'end_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:jobs:view',
        'retrieve': 'automation:jobs:view',
        'cancel': 'automation:jobs:cancel',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        status_value = self.request.query_params.get('status')  # type: ignore[union-attr]
        template_id = self.request.query_params.get('template_id')  # type: ignore[union-attr]
        keyword = self.request.query_params.get('keyword')  # type: ignore[union-attr]

        if status_value:
            queryset = queryset.filter(status=status_value)
        if template_id:
            queryset = queryset.filter(template_id=template_id)
        if keyword:
            queryset = queryset.filter(
                Q(job_id__icontains=keyword) |
                Q(requested_username__icontains=keyword) |
                Q(template__name__icontains=keyword)
            )

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
