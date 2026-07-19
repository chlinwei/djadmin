from __future__ import annotations
from .view_helpers import *
from .models import AutomationExecutionTargetLog


def _build_unified_log_text(job: AutomationExecutionJob) -> str:
    job_pk = int(getattr(job, 'pk', 0) or 0)
    rows = AutomationExecutionTargetLog.objects.filter(job_id=job_pk).order_by('id')
    chunks: list[str] = []
    for row in rows:
        chunks.append(
            f"\n\n===== Agent Host #{row.host_id_snapshot or '-'} ({row.host_ip_snapshot or '-'}) | status={row.status or 'unknown'} | job={row.agent_job_id or '-'} =====\n"
        )
        if row.stdout:
            chunks.append(str(row.stdout).rstrip('\n') + '\n')
        if row.stderr:
            chunks.append('[stderr]\n' + str(row.stderr).rstrip('\n') + '\n')
        if row.error_message:
            chunks.append('[error]\n' + str(row.error_message).rstrip('\n') + '\n')
    return ''.join(chunks)

class AutomationExecutionJobManage(GenericViewSet, RetrieveModelMixin, ListModelMixin):
    queryset = AutomationExecutionJob.objects.select_related('task').all()
    serializer_class = AutomationExecutionJobSerializer
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
        'target_logs': 'automation:jobs:view',
        'events': 'automation:jobs:view',
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
        output_keyword = self.request.query_params.get('output_keyword')  # type: ignore[union-attr]
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

        if output_keyword:
            # 统一日志从 target 明细聚合：按 stdout/stderr/error_message 三个维度搜索。
            queryset = queryset.filter(
                Q(target_logs__stdout__icontains=output_keyword)
                | Q(target_logs__stderr__icontains=output_keyword)
                | Q(target_logs__error_message__icontains=output_keyword)
            ).distinct()

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
                success = total_hosts if job_status == AutomationExecutionJob.Status.SUCCESS else 0
                failed = total_hosts if job_status == AutomationExecutionJob.Status.FAILED else 0
                pending = total_hosts if job_status in {AutomationExecutionJob.Status.PENDING, AutomationExecutionJob.Status.RUNNING} else 0
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
            'job_output': _build_unified_log_text(job),
        })

    @action(detail=True, methods=['get'])
    def target_logs(self, request, id=None):
        job = self.get_object()
        rows = job.target_logs.select_related('host').all().order_by('id')
        status_value = str(request.query_params.get('status') or '').strip().lower()
        host_id_value = str(request.query_params.get('host_id') or '').strip()

        if status_value:
            rows = rows.filter(status=status_value)
        if host_id_value.isdigit():
            rows = rows.filter(host_id_snapshot=int(host_id_value))

        data = []
        for row in rows:
            data.append({
                'id': row.id,
                'job_id': row.job_id,
                'host_id': row.host_id_snapshot,
                'host_name': row.host_name_snapshot,
                'host_ip': row.host_ip_snapshot,
                'agent_job_id': row.agent_job_id,
                'status': row.status,
                'exit_code': row.exit_code,
                'stdout': row.stdout or '',
                'stderr': row.stderr or '',
                'error_message': row.error_message or '',
                'result_data': row.result_data if isinstance(row.result_data, dict) else {},
                'create_time': row.create_time,
                'update_time': row.update_time,
            })

        return Response_200(data={
            'count': len(data),
            'results': data,
        })

    @action(detail=True, methods=['get'])
    def events(self, request, id=None):
        job = self.get_object()
        self.get_object()
        return Response_200(data=[])

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
        if job.status in {AutomationExecutionJob.Status.PENDING, AutomationExecutionJob.Status.RUNNING}:
            status_counts['pending'] = total_hosts
        elif job.status == AutomationExecutionJob.Status.SUCCESS:
            status_counts['success'] = total_hosts
        elif job.status == AutomationExecutionJob.Status.FAILED:
            status_counts['failed'] = total_hosts
        elif job.status == AutomationExecutionJob.Status.CANCELLED:
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
        if job.status in (AutomationExecutionJob.Status.SUCCESS, AutomationExecutionJob.Status.FAILED, AutomationExecutionJob.Status.CANCELLED):
            return Response_error_str('Job is already finished', code=400)

        now = timezone.now()
        if not job.start_time:
            job.start_time = now
        job.end_time = now
        if job.start_time:
            job.duration_seconds = (job.end_time - job.start_time).total_seconds()
        job.status = AutomationExecutionJob.Status.CANCELLED
        job.result_summary = {'message': 'Cancelled by user'}
        job.save(update_fields=['status', 'start_time', 'end_time', 'duration_seconds', 'result_summary'])

        return Response_200(data=AutomationExecutionJobSerializer(job).data)
