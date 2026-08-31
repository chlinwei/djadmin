from __future__ import annotations

from django.utils import timezone

from .view_helpers import *
from .view_helpers import _apply_limit_to_inventory_snapshot, _build_limit_matched_hosts_preview, _resolve_task_template
from .executor import execute_automation_job
from .executor_playbook import execute_playbook_job

class AutomationTaskManage(GenericViewSet, CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin):
    queryset = AutomationTask.objects.select_related('playbook_template', 'inventory').all()
    serializer_class = AutomationTaskSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'playbook_template__name', 'inventory__name', 'remark']
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

    def get_queryset(self):
        queryset = super().get_queryset()
        # 支持按 task_id 精确过滤，避免仅靠模糊 search 无法命中 ID。
        raw_task_id = str(self.request.query_params.get('task_id', '')).strip()  # type: ignore[union-attr]
        if raw_task_id.isdigit():
            queryset = queryset.filter(id=int(raw_task_id))
        return queryset


    def _run_now(self, task, task_template, user_info, started_at, inventory_snapshot, hosts,
                 extra_vars, limit_text):
        """立即执行 Playbook 任务。"""
        job = AutomationExecutionJob.objects.create(
            task=task,
            status=AutomationExecutionJob.Status.PENDING,
            trigger_type=AutomationExecutionJob.TriggerType.MANUAL,
            inventory_snapshot=inventory_snapshot,
            task_name_snapshot=task.name or '',
            template_name_snapshot=task_template.name or '',
            template_content_snapshot=task_template.content or '',
            extra_vars=extra_vars,
            limit=limit_text,
            requested_user_id=user_info.get('user_id'),
            requested_username=user_info.get('username', ''),
            result_summary={'message': 'Job created and queued for execution'},
            run_as_user_snapshot=task.run_as_user,
            run_as_group_snapshot=task.run_as_group,
            work_directory_snapshot=task.work_directory,
        )

        execute_automation_job(int(getattr(job, 'id', 0)))
        job.refresh_from_db()

        serializer = AutomationExecutionJobSerializer(job)
        return Response_200(data=serializer.data)

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
        inventory_name = ''
        if task.inventory_id and task.inventory is not None:
            default_host_ids = task.inventory.selected_host_ids
            inventory_name = task.inventory.name or ''

        limit_text = str(request.data.get('limit', task.default_limit or '')).strip()

        host_ids_raw = request.data.get('host_ids', default_host_ids)

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]

        if task.inventory_id and task.inventory is not None and len(host_ids) == 0:
            inventory_name = task.inventory.name or '-'
            return Response_200(data={
                'ok': False,
                'status': 'inventory_empty',
                'message': f'Inventory [{inventory_name}] 未选择主机，当前无可执行主机',
                'resolved_host_count': 0,
            })

        from assets.host_online import sync_host_online_status_to_db
        sync_host_online_status_to_db()

        inventory_snapshot = build_inventory_snapshot(host_ids=host_ids)
        inventory_snapshot = _apply_limit_to_inventory_snapshot(inventory_snapshot, limit_text)
        hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
        resolved_host_count = len(hosts) if isinstance(hosts, list) else 0
        if resolved_host_count == 0:
            if inventory_name:
                message = f'Inventory [{inventory_name}] 当前无可用主机，请检查主机是否被删除或范围配置是否正确'
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
        if task.inventory_id and task.inventory is not None:
            default_host_ids = task.inventory.selected_host_ids

        limit_text = str(request.data.get('limit', task.default_limit or '')).strip()

        host_ids_raw = request.data.get('host_ids', default_host_ids)
        extra_vars_raw = request.data.get('extra_vars', task.env_vars)

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        extra_vars = extra_vars_raw if isinstance(extra_vars_raw, dict) else {}

        if task.inventory_id and task.inventory is not None and len(host_ids) == 0:
            inventory_name = task.inventory.name or '-'
            return Response_error_str(
                f'Inventory [{inventory_name}] 未选择主机，当前无可执行主机',
                code=400,
            )

        inventory_snapshot = build_inventory_snapshot(host_ids=host_ids)
        inventory_snapshot = _apply_limit_to_inventory_snapshot(inventory_snapshot, limit_text)
        hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
        if not isinstance(hosts, list) or len(hosts) == 0:
            inventory_name = task.inventory.name if task.inventory_id and task.inventory else ''
            if inventory_name:
                return Response_error_str(
                    f'Inventory [{inventory_name}] 当前无可用主机，请检查主机是否被删除或范围配置是否正确',
                    code=400,
                )
            return Response_error_str('当前任务无可用主机，请检查执行范围配置', code=400)

        started_at = timezone.now()
        
        return self._run_now(
            task=task,
            task_template=task_template,
            user_info=user_info,
            started_at=started_at,
            inventory_snapshot=inventory_snapshot,
            hosts=hosts,
            extra_vars=extra_vars,
            limit_text=limit_text,
        )


