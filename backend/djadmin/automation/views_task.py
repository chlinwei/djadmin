from __future__ import annotations

import json
import os
import uuid
from urllib import error as urllib_error
from urllib import request as urllib_request

from django.conf import settings
from django.utils import timezone

from .view_helpers import *
from .view_helpers import _apply_limit_to_inventory_snapshot, _build_limit_matched_hosts_preview, _resolve_task_template
from .executor import execute_ansible_job
from assets.models import AgentJob, Host


def _build_agent_execute_url(host: Host) -> str:
    scheme = str(getattr(settings, 'AGENT_HTTP_SCHEME', os.getenv('AGENT_HTTP_SCHEME', 'http')) or 'http').strip() or 'http'
    port_text = str(getattr(settings, 'AGENT_HTTP_PORT', os.getenv('AGENT_HTTP_PORT', '19090')) or '19090').strip() or '19090'
    endpoint = str(
        getattr(settings, 'AGENT_HTTP_AUTOMATION_EXECUTE_ENDPOINT', os.getenv('AGENT_HTTP_AUTOMATION_EXECUTE_ENDPOINT', '/api/v1/automation/execute'))
        or '/api/v1/automation/execute'
    ).strip() or '/api/v1/automation/execute'
    if not endpoint.startswith('/'):
        endpoint = '/' + endpoint
    return f"{scheme}://{host.ip}:{port_text}{endpoint}"


def _resolve_agent_http_request_timeout(timeout_seconds: int) -> int:
    configured_timeout = getattr(settings, 'AGENT_HTTP_REQUEST_TIMEOUT_SECONDS', os.getenv('AGENT_HTTP_REQUEST_TIMEOUT_SECONDS', 15))
    try:
        configured_timeout_seconds = int(str(configured_timeout).strip())
    except (TypeError, ValueError):
        configured_timeout_seconds = 15

    effective_timeout_seconds = max(3, configured_timeout_seconds)
    # 上限保护：避免任务超时很大时导致请求阻塞过久（如 3600s+）。
    return min(effective_timeout_seconds, 30)


def _execute_automation_task_via_agent_http(host: Host, job_id: str, params: dict, timeout_seconds: int) -> dict:
    url = _build_agent_execute_url(host)
    payload = {
        'job_id': job_id,
        'type': 'custom',
        'action': 'run_automation_task',
        'params': params,
        'timeout_seconds': int(timeout_seconds),
    }
    request_body = json.dumps(payload, ensure_ascii=False).encode('utf-8')
    req = urllib_request.Request(url=url, data=request_body, method='POST')
    req.add_header('Content-Type', 'application/json')

    token = str(getattr(settings, 'AGENT_HTTP_TOKEN', os.getenv('DJ_AGENT_HTTP_TOKEN', '')) or '').strip()
    if token != '':
        req.add_header('Authorization', f'Bearer {token}')

    request_timeout = _resolve_agent_http_request_timeout(timeout_seconds)
    try:
        with urllib_request.urlopen(req, timeout=request_timeout) as resp:
            raw_text = resp.read().decode('utf-8', errors='replace')
            if int(resp.status) != 200:
                raise RuntimeError(f'agent http status={resp.status}: {raw_text}')
    except urllib_error.URLError as exc:
        raise RuntimeError(f'agent request failed: {exc}') from exc

    try:
        resp_payload = json.loads(raw_text)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f'agent response is not valid json: {raw_text}') from exc

    if int(resp_payload.get('code') or 0) != 200:
        raise RuntimeError(str(resp_payload.get('msg') or 'agent business error'))

    data = resp_payload.get('data')
    if not isinstance(data, dict):
        raise RuntimeError('agent response missing data')
    return data

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
        job = AnsibleExecutionJob.objects.create(
            task=task,
            status=AnsibleExecutionJob.Status.RUNNING,
            trigger_type=AnsibleExecutionJob.TriggerType.MANUAL,
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

        target_host_ids = [
            int(item.get('host_id'))
            for item in hosts
            if isinstance(item, dict) and str(item.get('host_id') or '').isdigit()
        ]
        host_map = {
            row.id: row
            for row in Host.objects.filter(id__in=target_host_ids)
        }

        created_count = 0
        success_count = 0
        failed_count = 0
        failed_rows = []
        output_chunks = []

        for item in hosts:
            if not isinstance(item, dict):
                continue
            host_id_raw = item.get('host_id')
            host_id_text = str(host_id_raw or '')
            if not host_id_text.isdigit():
                failed_count += 1
                failed_rows.append({'host_id': host_id_raw, 'error': 'invalid host_id'})
                continue

            host_id = int(host_id_text)
            host = host_map.get(host_id)
            if host is None:
                failed_count += 1
                failed_rows.append({'host_id': host_id, 'error': 'host not found'})
                continue

            agent_id = str(host.instance_name or '').strip()
            if agent_id == '':
                failed_count += 1
                failed_rows.append({'host_id': host_id, 'error': 'host has empty instance_name'})
                continue

            current_job_id = f'run_automation_task-{uuid.uuid4().hex[:16]}'
            current_params = {
                'template_type': 'shell_script' if is_shell_task else 'playbook',
                'template_content': task_template.content or '',
                'shell_parameters': shell_parameters if is_shell_task else '',
                'env_vars': shell_env_vars if is_shell_task else {},
                'extra_vars': extra_vars if not is_shell_task else {},
                'become_enabled': bool(task.become_enabled),
                'become_method': str(task.become_method or 'sudo'),
                'become_user': str(task.become_user or 'root'),
                'automation_execution_job_id': int(job.id),
                'automation_task_id': int(task.id),
                'host_id': int(host_id),
                'host_ip': str(host.ip or ''),
            }

            created_count += 1
            timeout_seconds = int(task.execution_timeout_seconds or 3600)
            try:
                exec_result = _execute_automation_task_via_agent_http(
                    host=host,
                    job_id=current_job_id,
                    params=current_params,
                    timeout_seconds=timeout_seconds,
                )
                current_status = str(exec_result.get('status') or '').strip().lower()
                stdout_text = str(exec_result.get('stdout') or '')
                stderr_text = str(exec_result.get('stderr') or '')
                error_text = str(exec_result.get('error_message') or '')

                if current_status == AgentJob.JobStatus.SUCCESS:
                    success_count += 1
                else:
                    failed_count += 1
                    failed_rows.append({
                        'host_id': host_id,
                        'error': error_text or f'agent status={current_status or "unknown"}',
                    })

                output_chunks.append(
                    f"\n\n===== Agent Host #{host_id} ({host.ip or '-'}) | status={current_status or 'unknown'} | job={current_job_id} =====\n"
                )
                if stdout_text:
                    output_chunks.append(stdout_text.rstrip('\n') + '\n')
                if stderr_text:
                    output_chunks.append('[stderr]\n' + stderr_text.rstrip('\n') + '\n')
                if error_text:
                    output_chunks.append('[error]\n' + error_text.rstrip('\n') + '\n')
            except Exception as exc:
                failed_count += 1
                failed_rows.append({'host_id': host_id, 'error': str(exc)})
                output_chunks.append(
                    f"\n\n===== Agent Host #{host_id} ({host.ip or '-'}) | status=failed | job={current_job_id} =====\n"
                )
                output_chunks.append('[error]\n' + str(exc).rstrip('\n') + '\n')

        if created_count == 0:
            finished_at = timezone.now()
            job.status = AnsibleExecutionJob.Status.FAILED
            job.end_time = finished_at
            job.duration_seconds = (finished_at - started_at).total_seconds()
            job.job_output = ''.join(output_chunks)
            job.result_summary = {
                'message': 'Failed to dispatch any agent task',
                'created_count': 0,
                'success_count': 0,
                'failed_count': failed_count,
                'failed_rows': failed_rows,
                'execution_mode': 'agent_http_sync',
            }
            job.save(update_fields=['status', 'result_summary', 'end_time', 'duration_seconds', 'job_output'])
            return Response_error_str('Job dispatch failed: no agent task created', code=400)

        finished_at = timezone.now()
        final_status = AnsibleExecutionJob.Status.SUCCESS if failed_count == 0 else AnsibleExecutionJob.Status.FAILED
        job.status = final_status
        job.end_time = finished_at
        job.duration_seconds = (finished_at - started_at).total_seconds()
        job.job_output = ''.join(output_chunks)
        job.result_summary = {
            'message': 'Job executed synchronously via agent http',
            'created_count': created_count,
            'success_count': success_count,
            'failed_count': failed_count,
            'failed_rows': failed_rows,
            'execution_mode': 'agent_http_sync',
        }
        job.save(update_fields=['status', 'result_summary', 'end_time', 'duration_seconds', 'job_output'])

        serializer = AnsibleExecutionJobSerializer(job)
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
        
        # 区分执行路径：Playbook 走本地，Shell 走 dj-agent
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
            # Playbook 模板：在本地 backend API 进程执行
            job = AnsibleExecutionJob.objects.create(
                task=task,
                status=AnsibleExecutionJob.Status.PENDING,
                trigger_type=AnsibleExecutionJob.TriggerType.MANUAL,
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
                result_summary={'message': 'Job created and executing locally in backend API'},
                start_time=started_at,
                # 权限提升配置快照
                become_enabled_snapshot=task.become_enabled,
                become_method_snapshot=task.become_method,
                become_user_snapshot=task.become_user,
            )
            
            try:
                # 直接在本进程执行
                execute_ansible_job(int(job.id))
            except Exception as exc:
                job.status = AnsibleExecutionJob.Status.FAILED
                job.result_summary = {'message': f'Failed to execute job in process: {str(exc)}'}
                job.save(update_fields=['status', 'result_summary'])
                return Response_error_str(f'Task execution failed: {str(exc)}', code=500)
            
            return Response_200(data={
                'job_id': job.id,
                'status': job.status,
                'result_summary': job.result_summary,
            })


