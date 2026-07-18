from __future__ import annotations
from .view_helpers import *
from .view_helpers import (
    _build_workflow_runtime_scope,
    _validate_workflow_task_nodes,
    _precheck_workflow_runtime_scope,
    _build_initial_node_results_from_nodes,
    _refresh_workflow_run_progress,
)

class AutomationWorkflowTemplateManage(
    GenericViewSet,
    CreateModelMixin,
    UpdateModelMixin,
    RetrieveModelMixin,
    ListModelMixin,
    DestroyModelMixin,
):
    queryset = AutomationWorkflowTemplate.objects.select_related('default_inventory').all()
    serializer_class = AutomationWorkflowTemplateSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'description', 'remark']
    ordering_fields = ['name', 'enabled', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:workflow:view',
        'retrieve': 'automation:workflow:view',
        'create': 'automation:workflow:create',
        'destroy': 'automation:workflow:delete',
        'partial_update': 'automation:workflow:update',
        'perform_update': 'automation:workflow:update',
        'launch': 'automation:workflow:launch',
        'precheck_launch': 'automation:workflow:view',
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
        try:
            instance = self.get_object()
            
            # 检查该 workflow 是否被其他 workflow 引用
            referenced_by = self._find_referencing_workflows(instance.id)
            if referenced_by:
                workflow_names = ', '.join([wf.name for wf in referenced_by])
                return Response(
                    {
                        'code': 600,
                        'msg': f'Workflow "{instance.name}" 被以下 Workflow 引用，无法删除：{workflow_names}',
                        'data': None
                    },
                    status=status.HTTP_400_BAD_REQUEST
                )
            
            deleted_id = instance.id
            # 删除 Workflow，WorkflowRun 记录会保留（设置 workflow_id 为 NULL）
            self.perform_destroy(instance)
            return Response_200(data={'id': deleted_id})
        except Exception as e:
            import logging
            logger = logging.getLogger(__name__)
            logger.exception(f"Error deleting workflow: {str(e)}")
            return Response(
                {
                    'code': 500,
                    'msg': f'删除失败: {str(e)}',
                    'data': None
                },
                status=status.HTTP_500_INTERNAL_SERVER_ERROR
            )

    def _find_referencing_workflows(self, workflow_id: int) -> list:
        """
        查找所有引用了指定 workflow 的其他 workflow。
        返回引用了该 workflow 的 workflow 列表。
        """
        # 从所有 workflow 中查找包含该 workflow_id 引用的
        all_workflows = AutomationWorkflowTemplate.objects.all()
        referencing = []
        
        for workflow in all_workflows:
            if workflow.id == workflow_id:
                continue  # 跳过自身
            
            # 检查 nodes 中是否有引用该 workflow 的节点
            if not workflow.nodes or not isinstance(workflow.nodes, list):
                continue  # nodes 为空或不是列表，跳过
            
            for node in workflow.nodes:
                if not isinstance(node, dict):
                    continue  # 节点不是字典，跳过
                
                try:
                    if (node.get('node_type') == 'workflow' and 
                        int(node.get('workflow_id', 0)) == workflow_id):
                        referencing.append(workflow)
                        break  # 找到一个引用就加入列表
                except (ValueError, TypeError):
                    # workflow_id 无法转换为 int，跳过该节点
                    continue
        
        return referencing

    @action(detail=True, methods=['post'], url_path='precheck-launch')
    def precheck_launch(self, request, id=None):
        workflow = self.get_object()
        if not workflow.enabled:
            return Response_200(data={
                'ok': False,
                'status': 'workflow_disabled',
                'message': 'Workflow 已禁用，无法启动',
                'resolved_host_count': 0,
                'effective_limit': '',
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
            })

        ok, error_msg, runtime_scope = _build_workflow_runtime_scope(workflow, request.data)
        if not ok or runtime_scope is None:
            return Response_200(data={
                'ok': False,
                'status': 'invalid_scope',
                'message': error_msg or '启动范围配置无效',
                'resolved_host_count': 0,
                'effective_limit': '',
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
            })

        workflow_nodes_snapshot = workflow.nodes if isinstance(workflow.nodes, list) else []
        nodes_ok, nodes_error = _validate_workflow_task_nodes(workflow_nodes_snapshot)
        if not nodes_ok:
            return Response_200(data={
                'ok': False,
                'status': 'node_task_invalid',
                'message': nodes_error or 'Workflow 节点任务配置无效',
                'resolved_host_count': 0,
                'effective_limit': str(runtime_scope.get('limit') or ''),
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
            })

        check_ok, check_status, check_data = _precheck_workflow_runtime_scope(runtime_scope)
        response_data = {
            'ok': check_ok,
            'status': check_status,
            'message': check_data.get('message') or '',
            'resolved_host_count': check_data.get('resolved_host_count') or 0,
            'effective_limit': check_data.get('effective_limit') or '',
            'matched_hosts_preview': check_data.get('matched_hosts_preview') or [],
            'matched_hosts_preview_total': check_data.get('matched_hosts_preview_total') or 0,
            'use_global_scope': bool(runtime_scope.get('use_global_scope')),
            'inventory_id': runtime_scope.get('inventory_id'),
            'inventory_name': runtime_scope.get('inventory_name') or '',
        }
        if 'missing_group_ids' in check_data:
            response_data['missing_group_ids'] = check_data.get('missing_group_ids') or []

        return Response_200(data=response_data)

    @action(detail=True, methods=['post'], url_path='launch')
    def launch(self, request, id=None):
        workflow = self.get_object()
        if not workflow.enabled:
            return Response_error_str('Workflow is disabled', code=400)

        user_info = getCurrentUser(request)
        outcome_map = request.data.get('node_outcomes', {})
        extra_vars = request.data.get('extra_vars', workflow.default_extra_vars)

        if outcome_map is not None and not isinstance(outcome_map, dict):
            return Response_error_str('node_outcomes must be an object', code=400)
        if not isinstance(extra_vars, dict):
            return Response_error_str('extra_vars must be an object', code=400)

        workflow_nodes_snapshot = workflow.nodes if isinstance(workflow.nodes, list) else []
        if len(workflow_nodes_snapshot) == 0:
            return Response_error_str('Workflow plan is empty, please check nodes and edges', code=400)

        nodes_ok, nodes_error = _validate_workflow_task_nodes(workflow_nodes_snapshot)
        if not nodes_ok:
            return Response_error_str(nodes_error or 'Workflow task nodes are invalid', code=400)

        ok, error_msg, runtime_scope = _build_workflow_runtime_scope(workflow, request.data)
        if not ok or runtime_scope is None:
            return Response_error_str(error_msg or 'Workflow runtime scope is invalid', code=400)

        check_ok, _, check_data = _precheck_workflow_runtime_scope(runtime_scope)
        if not check_ok:
            return Response_error_str(check_data.get('message') or 'Workflow runtime scope precheck failed', code=400)

        run = AutomationWorkflowRun.objects.create(
            workflow=workflow,
            status=AutomationWorkflowRun.Status.RUNNING,
            trigger_type=AutomationWorkflowRun.TriggerType.MANUAL,
            workflow_name_snapshot=workflow.name or '',
            workflow_code_snapshot='',
            planned_node_keys=[str(item.get('key') or '').strip() for item in workflow_nodes_snapshot if str(item.get('key') or '').strip()],
            node_results=[],
            extra_vars=extra_vars,
            result_summary={'message': 'Workflow run created'},
            requested_user_id=user_info.get('user_id'),
            requested_username=user_info.get('username', ''),
            start_time=timezone.now(),
        )

        node_results = _build_initial_node_results_from_nodes(workflow_nodes_snapshot)

        run.node_results = node_results
        workflow_edges_snapshot = workflow.edges if isinstance(workflow.edges, list) else []
        run.result_summary = {
            'message': 'Workflow run created',
            'queued_job_count': 0,
            'queued_job_ids': [],
            'workflow_ancestor_template_ids': [workflow.id],
            'runtime_scope': runtime_scope,
            # Keep immutable graph snapshot for this run so later workflow edits do not affect history.
            'workflow_nodes_snapshot': json.loads(json.dumps(workflow_nodes_snapshot, ensure_ascii=False)),
            'workflow_edges_snapshot': json.loads(json.dumps(workflow_edges_snapshot, ensure_ascii=False)),
        }
        run.save(update_fields=['node_results', 'result_summary'])
        _refresh_workflow_run_progress(run)

        return Response_200(data=AutomationWorkflowRunSerializer(run).data)


class AutomationWorkflowRunManage(GenericViewSet, RetrieveModelMixin, ListModelMixin):
    queryset = AutomationWorkflowRun.objects.select_related('workflow').all()
    serializer_class = AutomationWorkflowRunSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['workflow_name_snapshot', 'requested_username', 'remark']
    ordering_fields = ['id', 'status', 'duration_seconds', 'create_time', 'update_time', 'start_time', 'end_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:workflow:view',
        'retrieve': 'automation:workflow:view',
        'cancel': 'automation:jobs:cancel',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        params = self.request.query_params  # type: ignore[union-attr]

        workflow_id = params.get('workflow_id')
        if workflow_id and str(workflow_id).isdigit():
            queryset = queryset.filter(workflow_id=int(workflow_id))

        status_value = params.get('status')
        if status_value:
            queryset = queryset.filter(status=status_value)

        requested_username = str(params.get('requested_username') or '').strip()
        if requested_username:
            queryset = queryset.filter(requested_username__icontains=requested_username)

        start_time_after = str(params.get('start_time_after') or '').strip()
        if start_time_after:
            queryset = queryset.filter(start_time__gte=start_time_after)

        start_time_before = str(params.get('start_time_before') or '').strip()
        if start_time_before:
            queryset = queryset.filter(start_time__lte=start_time_before)

        return queryset

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        # 注：dj-agent 已负责异步更新 workflow run 状态，无需在列表查询时逐个调用 _refresh_workflow_run_progress
        # 该操作会导致大量 N+1 数据库查询，造成主 API 卡死。仅在 retrieve (详情) 时刷新最新状态。
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
        _refresh_workflow_run_progress(instance)
        serializer = self.get_serializer(instance)
        return Response_200(data=serializer.data)

    @action(detail=True, methods=['post'])
    def cancel(self, request, id=None):
        run = self.get_object()
        if run.status in (AutomationWorkflowRun.Status.SUCCESS, AutomationWorkflowRun.Status.FAILED):
            return Response_error_str('Workflow run is already finished', code=400)

        now = timezone.now()
        node_results = [dict(item) for item in (run.node_results or []) if isinstance(item, dict)]
        job_ids = [int(item['job_id']) for item in node_results if str(item.get('job_id', '')).isdigit()]
        cancel_job_ids = []

        if job_ids:
            jobs = AnsibleExecutionJob.objects.filter(
                id__in=list(set(job_ids)),
                status__in=[AnsibleExecutionJob.Status.PENDING, AnsibleExecutionJob.Status.RUNNING],
            )

            for job in jobs:
                if not job.start_time:
                    job.start_time = now
                job.end_time = now
                job.duration_seconds = (job.end_time - job.start_time).total_seconds() if job.start_time else None
                job.status = AnsibleExecutionJob.Status.CANCELLED
                summary = job.result_summary if isinstance(job.result_summary, dict) else {}
                summary['message'] = 'Cancelled by workflow run cancellation'
                job.result_summary = summary
                job.save(update_fields=['status', 'start_time', 'end_time', 'duration_seconds', 'result_summary'])
                cancel_job_ids.append(job.id)

        cancelled_job_id_set = set(cancel_job_ids)
        for item in node_results:
            node_status = str(item.get('status') or '').lower()
            node_job_id = int(item['job_id']) if str(item.get('job_id', '')).isdigit() else None

            if node_job_id in cancelled_job_id_set:
                item['status'] = 'cancelled'
                item['message'] = 'Workflow run cancelled by user'
                continue

            if node_status in {'waiting', 'pending', 'queued', 'running'} and node_job_id is None:
                item['status'] = 'skipped'
                item['message'] = 'Workflow run cancelled by user'

        summary = run.result_summary if isinstance(run.result_summary, dict) else {}
        summary['cancelled'] = True
        summary['cancelled_at'] = now.isoformat()
        summary['cancelled_by'] = str(request.user)
        summary['cancelled_job_count'] = len(cancel_job_ids)
        summary['cancelled_job_ids'] = cancel_job_ids
        summary['message'] = 'Workflow run cancelled by user'

        if not run.start_time:
            run.start_time = now
        run.end_time = now
        run.duration_seconds = (run.end_time - run.start_time).total_seconds() if run.start_time else None
        run.status = AutomationWorkflowRun.Status.FAILED
        run.node_results = node_results
        run.result_summary = summary
        run.save(update_fields=['status', 'node_results', 'result_summary', 'start_time', 'end_time', 'duration_seconds'])

        _refresh_workflow_run_progress(run)
        serializer = self.get_serializer(run)
        return Response_200(data=serializer.data)


