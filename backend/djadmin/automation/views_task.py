from __future__ import annotations

from django.utils import timezone

from .view_helpers import *
from .view_helpers import _apply_limit_to_inventory_snapshot, _build_limit_matched_hosts_preview, _resolve_task_template
from .local_runner import run_job_in_background
from .agent_http_runner import execute_job_via_agent_http

class AutomationTaskManage(GenericViewSet, CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin):
    queryset = AutomationTask.objects.select_related('playbook_template', 'shell_script_template', 'inventory').all()
    serializer_class = AutomationTaskSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'playbook_template__name', 'shell_script_template__name', 'inventory__name', 'remark']
    ordering_fields = ['name', 'enabled', 'create_time', 'update_time']
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

    def _run_now_via_agent(self, task, task_template, user_info, started_at, inventory_snapshot, hosts, 
                           is_shell_task, extra_vars, shell_parameters, shell_env_vars, limit_text):
        """通过 dj-agent HTTP 执行 shell 任务。"""
        job = AutomationExecutionJob.objects.create(
            task=task,
            status=AutomationExecutionJob.Status.RUNNING,
            trigger_type=AutomationExecutionJob.TriggerType.MANUAL,
            inventory_snapshot=inventory_snapshot,
            task_name_snapshot=task.name or '',
            template_name_snapshot=task_template.name or '',
            template_content_snapshot=task_template.content or '',
            extra_vars=extra_vars if not is_shell_task else {},
            shell_parameters=shell_parameters if is_shell_task else '',
            shell_env_vars=shell_env_vars if is_shell_task else {},
            limit=limit_text,
            requested_user_id=user_info.get('user_id'),
            requested_username=user_info.get('username', ''),
            result_summary={'message': 'Job created and executing via agent http'},
            start_time=started_at,
            # 权限提升配置快照
            become_enabled_snapshot=task.become_enabled,
            become_method_snapshot=task.become_method,
            become_user_snapshot=task.become_user,
        )

        job_pk = int(getattr(job, 'pk', 0) or 0)
        task_pk = int(getattr(task, 'pk', 0) or 0)

        success, summary, _ = execute_job_via_agent_http(
            automation_execution_job_id=job_pk,
            automation_task_id=task_pk,
            template_content=job.template_content_snapshot or '',
            template_type='shell_script' if is_shell_task else 'playbook',
            hosts=hosts,
            shell_parameters=shell_parameters if is_shell_task else '',
            shell_env_vars=shell_env_vars if is_shell_task else {},
            extra_vars=extra_vars if not is_shell_task else {},
            become_enabled=bool(task.become_enabled),
            become_method=str(task.become_method or 'sudo'),
            become_user=str(task.become_user or 'root'),
            timeout_seconds=int(task.execution_timeout_seconds or 600),
        )

        if int(summary.get('created_count', 0) or 0) == 0:
            finished_at = timezone.now()
            job.status = AutomationExecutionJob.Status.FAILED
            job.end_time = finished_at
            job.duration_seconds = (finished_at - started_at).total_seconds()
            job.result_summary = summary
            job.save(update_fields=['status', 'result_summary', 'end_time', 'duration_seconds'])
            return Response_error_str('Job dispatch failed: no agent task created', code=400)

        finished_at = timezone.now()
        final_status = AutomationExecutionJob.Status.SUCCESS if success else AutomationExecutionJob.Status.FAILED
        job.status = final_status
        job.end_time = finished_at
        job.duration_seconds = (finished_at - started_at).total_seconds()
        job.result_summary = summary
        job.save(update_fields=['status', 'result_summary', 'end_time', 'duration_seconds'])

        serializer = AutomationExecutionJobSerializer(job)
        return Response_200(data=serializer.data)

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        # 支持按 task_id 精确过滤，避免仅靠模糊 search 无法命中 ID。
        raw_task_id = str(request.query_params.get('task_id', '')).strip()
        if raw_task_id.isdigit():
            queryset = queryset.filter(id=int(raw_task_id))
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
        task_id = instance.id
        
        # 检查该任务是否被 workflow 引用
        workflows_with_task = []
        for workflow in AutomationWorkflowTemplate.objects.all():
            nodes = workflow.nodes or []
            for node in nodes:
                # task 节点的 task_id 字段存储引用
                if isinstance(node, dict) and node.get('node_type') == 'task' and node.get('task_id') == task_id:
                    workflows_with_task.append(workflow.name)
                    break
        
        if workflows_with_task:
            workflow_names = ', '.join(workflows_with_task)
            return Response_error_str(
                f'任务被以下工作流引用，无法删除: {workflow_names}',
                code=400
            )
        
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

        if _resolve_task_template(task) is None:
            return Response_200(data={
                'ok': False,
                'status': 'template_missing',
                'message': '任务模板不存在',
                'resolved_host_count': 0,
            })

        if not task.inventory_id:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_missing',
                'message': '任务未配置 Inventory，无法预检执行范围',
                'resolved_host_count': 0,
            })

        if task.inventory_id and task.inventory is not None and not task.inventory.enabled:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_disabled',
                'message': 'Inventory 已禁用，无法执行',
                'resolved_host_count': 0,
            })

        default_host_ids = []
        default_group_ids = []
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

        hosts_list = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
        offline_count = sum(1 for h in hosts_list if isinstance(h, dict) and not h.get('agent_online'))

        if offline_count > 0:
            return Response_200(data={
                'ok': False,
                'status': 'has_offline_hosts',
                'message': f'有 {offline_count} 台主机 Agent 离线，请确保所有目标主机在线后再执行',
                'resolved_host_count': resolved_host_count,
                'offline_hosts_count': offline_count,
                'effective_limit': limit_text,
                'matched_hosts_preview': _build_limit_matched_hosts_preview(inventory_snapshot),
                'matched_hosts_preview_total': resolved_host_count,
            })

        return Response_200(data={
            'ok': True,
            'status': 'ok',
            'message': f'预检通过，可执行主机 {resolved_host_count} 台',
            'resolved_host_count': resolved_host_count,
            'offline_hosts_count': 0,
            'effective_limit': limit_text,
            'matched_hosts_preview': _build_limit_matched_hosts_preview(inventory_snapshot),
            'matched_hosts_preview_total': resolved_host_count,
        })

    @action(detail=True, methods=['post'], url_path='run_now')
    def run_now(self, request, id=None):
        task = self.get_object()
        task_template = _resolve_task_template(task)
        if not task.enabled:
            return Response_error_str('Task is disabled', code=400)
        if task_template is None:
            return Response_error_str('Template is missing', code=400)
        # inventory 是必选的，未配置或已被删除时拒绝执行
        if not task.inventory_id:
            return Response_error_str('Task 未配置 Inventory，无法执行', code=400)
        if task.inventory_id and task.inventory is not None and not task.inventory.enabled:
            return Response_error_str('Inventory is disabled', code=400)

        user_info = getCurrentUser(request)
        default_host_ids = []
        default_group_ids = []
        if task.inventory_id and task.inventory is not None:
            default_host_ids = task.inventory.selected_host_ids
            default_group_ids = task.inventory.selected_group_ids

        limit_text = str(request.data.get('limit', task.default_limit or '')).strip()

        host_ids_raw = request.data.get('host_ids', default_host_ids)
        group_ids_raw = request.data.get('group_ids', default_group_ids)
        is_shell_task = task.shell_script_template_id is not None
        extra_vars_raw = request.data.get('extra_vars', task.env_vars if not is_shell_task else {})
        shell_parameters_raw = request.data.get('shell_parameters', task.shell_parameters if is_shell_task else '')
        shell_env_vars_raw = request.data.get('shell_env_vars', task.env_vars if is_shell_task else {})

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        group_ids = group_ids_raw if isinstance(group_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        group_ids = [int(item) for item in group_ids if str(item).isdigit()]
        extra_vars = extra_vars_raw if isinstance(extra_vars_raw, dict) else {}
        shell_parameters = str(shell_parameters_raw or '').strip()
        shell_env_vars = shell_env_vars_raw if isinstance(shell_env_vars_raw, dict) else {}

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

        started_at = timezone.now()
        
        # 区分执行路径：Shell 直接经由 agent；Playbook 由后台 runner 统一转发到 agent 执行
        if is_shell_task:
            # Shell 模板：通过 dj-agent HTTP 执行
            return self._run_now_via_agent(
                task=task,
                task_template=task_template,
                user_info=user_info,
                started_at=started_at,
                inventory_snapshot=inventory_snapshot,
                hosts=hosts,
                is_shell_task=is_shell_task,
                extra_vars=extra_vars,
                shell_parameters=shell_parameters,
                shell_env_vars=shell_env_vars,
                limit_text=limit_text,
            )
        else:
            # Playbook 模板：创建作业后由后台 runner 转发到各主机 agent 执行
            job = AutomationExecutionJob.objects.create(
                task=task,
                status=AutomationExecutionJob.Status.PENDING,
                trigger_type=AutomationExecutionJob.TriggerType.MANUAL,
                inventory_snapshot=inventory_snapshot,
                task_name_snapshot=task.name or '',
                template_name_snapshot=task_template.name or '',
                template_content_snapshot=task_template.content or '',
                extra_vars=extra_vars,
                shell_parameters='',
                shell_env_vars={},
                limit=limit_text,
                requested_user_id=user_info.get('user_id'),
                requested_username=user_info.get('username', ''),
                result_summary={'message': 'Job created and waiting for backend runner to dispatch via agent HTTP'},
                start_time=started_at,
                # 权限提升配置快照
                become_enabled_snapshot=task.become_enabled,
                become_method_snapshot=task.become_method,
                become_user_snapshot=task.become_user,
            )
            
            try:
                # 非 Celery：改为本地后台线程执行，避免阻塞 API 响应。
                run_job_in_background(int(job.id))
            except Exception as exc:
                job.status = AutomationExecutionJob.Status.FAILED
                job.result_summary = {'message': f'Failed to start local runner: {str(exc)}'}
                job.save(update_fields=['status', 'result_summary'])
                return Response_error_str(f'Task execution failed: {str(exc)}', code=500)
            
            serializer = AutomationExecutionJobSerializer(job)
            return Response_200(data=serializer.data)


