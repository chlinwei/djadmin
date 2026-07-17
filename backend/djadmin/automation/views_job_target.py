from __future__ import annotations
from .view_helpers import *

class AnsibleExecutionJobManage(GenericViewSet, RetrieveModelMixin, ListModelMixin):
    queryset = AnsibleExecutionJob.objects.select_related('task').all()
    serializer_class = AnsibleExecutionJobSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['job_id', 'requested_username', 'template_name_snapshot', 'task_name_snapshot', 'remark']
    ordering_fields = ['id', 'status', 'duration_seconds', 'create_time', 'update_time', 'start_time', 'end_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:jobs:view',
        'retrieve': 'automation:jobs:view',
        'cancel': 'automation:jobs:cancel',
        'log': 'automation:jobs:view',
        'events': 'automation:jobs:view',
        'host_summary': 'automation:jobs:view',
        'status_summary': 'automation:jobs:view',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        if self.action != 'list':
            return queryset

        status_value = self.request.query_params.get('status')  # type: ignore[union-attr]
        task_id = self.request.query_params.get('task_id')  # type: ignore[union-attr]
        job_id = self.request.query_params.get('job_id')  # type: ignore[union-attr]
        keyword = self.request.query_params.get('keyword')  # type: ignore[union-attr]
        start_time_from = self.request.query_params.get('start_time_from')  # type: ignore[union-attr]
        start_time_to = self.request.query_params.get('start_time_to')  # type: ignore[union-attr]

        if status_value:
            queryset = queryset.filter(status=status_value)
        if task_id:
            queryset = queryset.filter(task_id=task_id)
        if job_id:
            # 按执行记录 ID 精确查询
            if str(job_id).isdigit():
                queryset = queryset.filter(id=int(job_id))
        elif keyword:
            condition = (
                Q(requested_username__icontains=keyword) |
                Q(template_name_snapshot__icontains=keyword) |
                Q(task_name_snapshot__icontains=keyword)
            )
            queryset = queryset.filter(condition)

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

        include_status_summary = str(request.query_params.get('include_status_summary') or '').strip().lower()
        if include_status_summary in {'1', 'true', 'yes', 'on'}:
            for item in data:
                inventory_snapshot = item.get('inventory_snapshot') if isinstance(item.get('inventory_snapshot'), dict) else {}
                hosts = inventory_snapshot.get('hosts') if isinstance(inventory_snapshot, dict) else []
                total_hosts = len(hosts) if isinstance(hosts, list) else 0
                job_status = str(item.get('status') or '').lower()
                success = total_hosts if job_status == AnsibleExecutionJob.Status.SUCCESS else 0
                failed = total_hosts if job_status == AnsibleExecutionJob.Status.FAILED else 0
                pending = total_hosts if job_status in {AnsibleExecutionJob.Status.PENDING, AnsibleExecutionJob.Status.RUNNING} else 0
                item['status_summary'] = {
                    'total_hosts': total_hosts,
                    'finished_hosts': success + failed,
                    'pending': pending,
                    'running': 0,
                    'success': success,
                    'failed': failed,
                    'skipped': 0,
                    'unreachable': 0,
                }
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

    @action(detail=True, methods=['get'])
    def events(self, request, id=None):
        job = self.get_object()
        self.get_object()
        return Response_200(data=[])

    @action(detail=True, methods=['get'])
    def host_summary(self, request, id=None):
        job = self.get_object()
        inventory_snapshot = job.inventory_snapshot if isinstance(job.inventory_snapshot, dict) else {}
        hosts = inventory_snapshot.get('hosts') if isinstance(inventory_snapshot, dict) else []
        host_rows = hosts if isinstance(hosts, list) else []
        summary_list: list[dict[str, Any]] = []
        for row in host_rows:
            if not isinstance(row, dict):
                continue
            summary_list.append({
                'target_id': None,
                'host_name': str(row.get('host_name') or '').strip(),
                'host_ip': str(row.get('host_ip') or '').strip(),
                'status': str(job.status or '').strip(),
                'rc': None,
                'duration_seconds': job.duration_seconds,
                'total_events': 0,
                'stdout_events': 0,
                'stderr_events': 0,
                'last_line_no': 0,
            })


        host_name = str(request.query_params.get('host_name') or '').strip().lower()
        host_ip = str(request.query_params.get('host_ip') or '').strip().lower()
        status_value = str(request.query_params.get('status') or '').strip().lower()

        if host_name:
            summary_list = [
                item for item in summary_list
                if host_name in str(item.get('host_name') or '').lower()
            ]
        if host_ip:
            summary_list = [
                item for item in summary_list
                if host_ip in str(item.get('host_ip') or '').lower()
            ]
        if status_value:
            summary_list = [
                item for item in summary_list
                if status_value == str(item.get('status') or '').lower()
            ]

        summary_list.sort(
            key=lambda item: (
                int(item.get('target_id') or 0),
                str(item.get('host_name') or ''),
                str(item.get('host_ip') or ''),
            )
        )

        page = self.paginate_queryset(summary_list)
        page_data = page if page is not None else summary_list
        if page is not None:
            paginator = self.paginator
            return Response_200(data={
                'count': paginator.page.paginator.count,
                'results': page_data,
                'pageNumber': paginator.page.number,
                'pageSize': paginator.page_size,
                'totalPages': paginator.page.paginator.num_pages,
                'next': paginator.get_next_link(),
                'previous': paginator.get_previous_link(),
            })
        return Response_200(data=page_data)

    @action(detail=True, methods=['get'])
    def status_summary(self, request, id=None):
        job = self.get_object()
        inventory_snapshot = job.inventory_snapshot if isinstance(job.inventory_snapshot, dict) else {}
        hosts = inventory_snapshot.get('hosts') if isinstance(inventory_snapshot, dict) else []
        total_hosts = len(hosts) if isinstance(hosts, list) else 0
        status_counts = {
            'pending': 0,
            'running': 0,
            'success': 0,
            'failed': 0,
            'skipped': 0,
            'unreachable': 0,
        }
        if job.status in {AnsibleExecutionJob.Status.PENDING, AnsibleExecutionJob.Status.RUNNING}:
            status_counts['pending'] = total_hosts
        elif job.status == AnsibleExecutionJob.Status.SUCCESS:
            status_counts['success'] = total_hosts
        elif job.status == AnsibleExecutionJob.Status.FAILED:
            status_counts['failed'] = total_hosts
        elif job.status == AnsibleExecutionJob.Status.CANCELLED:
            status_counts['skipped'] = total_hosts

        finished_hosts = total_hosts - status_counts['pending'] - status_counts['running']

        return Response_200(data={
            'job_id': job.id,
            'job_status': job.status,
            'total_hosts': total_hosts,
            'finished_hosts': finished_hosts,
            'pending': status_counts['pending'],
            'running': status_counts['running'],
            'success': status_counts['success'],
            'failed': status_counts['failed'],
            'skipped': status_counts['skipped'],
            'unreachable': status_counts['unreachable'],
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

        return Response_200(data=AnsibleExecutionJobSerializer(job).data)
