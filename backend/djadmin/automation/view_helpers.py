from __future__ import annotations
import json
import os
import re
import fnmatch
from typing import Any
from urllib.parse import quote

from django.http import HttpResponse
from django.utils.dateparse import parse_datetime
from django.utils import timezone
from django.db import transaction
from django.db.models import Count, Max, Q
from django_filters.rest_framework import DjangoFilterBackend
from rest_framework.decorators import action
from rest_framework.filters import OrderingFilter, SearchFilter
from rest_framework.mixins import CreateModelMixin, DestroyModelMixin, ListModelMixin, RetrieveModelMixin, UpdateModelMixin
from rest_framework import serializers, status
from rest_framework.viewsets import GenericViewSet
from rest_framework.response import Response

from djadmin.utils import CustomPagination, Response_200, Response_error_str
from menu.permisssion import CustomMenuPermission
from user.utils import getCurrentUser
from assets.models import Host, HostGroup

from .models import (
    PlaybookTemplate,
    ShellScriptTemplate,
    AutomationTask,
    AutomationInventory,
    AutomationExecutionJob,
    AutomationWorkflowTemplate,
    AutomationWorkflowRun,
)
from .serializer import (
    PlaybookTemplateSerializer,
    ShellScriptTemplateSerializer,
    AutomationTaskSerializer,
    AutomationInventorySerializer,
    AutomationExecutionJobSerializer,
    AutomationWorkflowTemplateSerializer,
    AutomationWorkflowRunSerializer,
    validate_playbook_content_or_raise,
    check_workflow_cycle_at_runtime,
)
from .executor import build_inventory_snapshot
from .local_runner import run_job_in_background
from .workflow_runtime import WORKFLOW_RUNTIME_FINAL_STATUSES, get_workflow_runtime_status


def _resolve_task_template(task: AutomationTask):
    return task.playbook_template or task.shell_script_template


def _build_initial_node_results_from_nodes(nodes: list[dict]) -> list[dict]:
    task_ids = {
        int(str(node.get('task_id') or '0'))
        for node in nodes
        if isinstance(node, dict)
        and str(node.get('task_id', '')).isdigit()
        and str(node.get('node_type') or '').strip().lower() == 'task'
    }
    workflow_ids = {
        int(str(node.get('workflow_id') or '0'))
        for node in nodes
        if isinstance(node, dict)
        and str(node.get('workflow_id', '')).isdigit()
        and str(node.get('node_type') or '').strip().lower() == 'workflow'
    }

    task_snapshot_map = {}
    if task_ids:
        rows = AutomationTask.objects.filter(id__in=list(task_ids)).select_related('playbook_template', 'shell_script_template')
        for row in rows:
            task_template = _resolve_task_template(row)
            task_id = int(getattr(row, 'pk'))
            task_snapshot_map[task_id] = {
                'task_name_snapshot': str(getattr(row, 'name', '') or '').strip(),
                'job_template_id': int(task_template.id) if task_template is not None else None,
                'template_name_snapshot': str(task_template.name or '').strip() if task_template is not None else '',
            }

    workflow_snapshot_map = {}
    if workflow_ids:
        rows = AutomationWorkflowTemplate.objects.filter(id__in=list(workflow_ids)).values('id', 'name')
        workflow_snapshot_map = {int(row['id']): str(row.get('name') or '').strip() for row in rows}

    initial_results = []
    for node in nodes:
        if not isinstance(node, dict):
            continue
        node_key = str(node.get('key') or '').strip()
        if not node_key:
            continue
        node_type = str(node.get('node_type') or 'task').strip().lower() or 'task'
        task_id_text = str(node.get('task_id', '') or '').strip()
        workflow_id_text = str(node.get('workflow_id', '') or '').strip()
        task_id_value = int(task_id_text) if task_id_text.isdigit() else None
        workflow_id_value = int(workflow_id_text) if workflow_id_text.isdigit() else None
        task_snapshot = task_snapshot_map.get(task_id_value, {}) if task_id_value else {}

        initial_results.append({
            'node_key': node_key,
            'node_name': str(node.get('name') or node_key),
            'node_type': node_type,
            'convergence': str(node.get('convergence') or 'any').strip().lower() or 'any',
            'status': 'waiting',
            'task_id': task_id_value,
            'workflow_id': workflow_id_value,
            # 运行时快照：即使后续任务/模板被删除，历史 run 仍可回放节点引用信息。
            'task_name_snapshot': str(node.get('task_name') or task_snapshot.get('task_name_snapshot') or '').strip(),
            'job_template_id': task_snapshot.get('job_template_id'),
            'template_name_snapshot': str(node.get('template_name') or task_snapshot.get('template_name_snapshot') or '').strip(),
            'workflow_name_snapshot': str(
                node.get('workflow_name')
                or (workflow_snapshot_map.get(workflow_id_value, '') if workflow_id_value else '')
                or ''
            ).strip(),
        })
    return initial_results


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
    raw_token = str(token or '').strip().lower()
    if not raw_token:
        return False

    scope = ''
    has_scope = False
    pattern = raw_token
    if ':' in raw_token:
        has_scope = True
        scope, pattern = raw_token.split(':', 1)
        scope = scope.strip()
        pattern = pattern.strip()
        if not pattern:
            return False

    host_id_text = str(host_item.get('host_id') or '')
    host_name = str(host_item.get('host_name') or '').lower()
    host_ip = str(host_item.get('host_ip') or '').lower()
    group_name = str(host_item.get('group_name') or '').lower()
    group_path = str(host_item.get('group_path') or '').lower()

    if scope in ('host', 'hostname', 'name'):
        return fnmatch.fnmatch(host_name, pattern)
    if scope in ('id', 'host_id'):
        return fnmatch.fnmatch(host_id_text, pattern)
    if scope in ('path', 'group_path'):
        return fnmatch.fnmatch(group_path, pattern)
    if has_scope:
        return False

    if fnmatch.fnmatch(host_id_text, pattern):
        return True
    if fnmatch.fnmatch(host_ip, pattern):
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
            'agent_online': bool(item.get('agent_online')),
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


def _read_workflow_ancestor_template_ids(run: AutomationWorkflowRun) -> list[int]:
    # 祖先链保存在当前 run 的 summary 中，用于运行时递归检测。
    result_summary = run.result_summary if isinstance(run.result_summary, dict) else {}
    raw_ids = result_summary.get('workflow_ancestor_template_ids', [])
    ancestor_ids = []
    if isinstance(raw_ids, list):
        for item in raw_ids:
            if not str(item).isdigit():
                continue
            workflow_id = int(item)
            if workflow_id > 0 and workflow_id not in ancestor_ids:
                ancestor_ids.append(workflow_id)

    if run.workflow_id and int(run.workflow_id) > 0 and int(run.workflow_id) not in ancestor_ids:
        ancestor_ids.append(int(run.workflow_id))

    return ancestor_ids


def _format_workflow_spawn_chain_display(spawn_workflow_id: int, ancestor_template_ids: list[int]) -> str:
    # 生成便于排障的链路文案：当前目标 -> 最近父级 -> 更上层祖先。
    display_ids = [spawn_workflow_id]
    display_ids.extend(reversed([item for item in ancestor_template_ids if item != spawn_workflow_id]))

    template_rows = AutomationWorkflowTemplate.objects.filter(id__in=display_ids).values('id', 'name')
    template_name_map = {int(item['id']): str(item.get('name') or '') for item in template_rows}

    display_parts = []
    for workflow_id in display_ids:
        workflow_name = template_name_map.get(workflow_id, '')
        if workflow_name:
            display_parts.append(f"{workflow_name}#{workflow_id}")
        else:
            display_parts.append(f"#{workflow_id}")

    return ' -> '.join(display_parts)


def _extract_workflow_runtime_scope(run: AutomationWorkflowRun) -> dict:
    result_summary = run.result_summary if isinstance(run.result_summary, dict) else {}
    runtime_scope = result_summary.get('runtime_scope', {})
    if not isinstance(runtime_scope, dict):
        runtime_scope = {}

    host_ids = [int(item) for item in (runtime_scope.get('host_ids') or []) if str(item).isdigit()]
    group_ids = [int(item) for item in (runtime_scope.get('group_ids') or []) if str(item).isdigit()]

    return {
        'use_global_scope': bool(runtime_scope.get('use_global_scope')),
        'inventory_id': int(runtime_scope['inventory_id']) if str(runtime_scope.get('inventory_id', '')).isdigit() else None,
        'inventory_name': str(runtime_scope.get('inventory_name') or ''),
        'host_ids': host_ids,
        'group_ids': group_ids,
        'limit': str(runtime_scope.get('limit') or '').strip(),
        'limit_locked': bool(runtime_scope.get('limit_locked')),
    }


def _build_workflow_runtime_scope(workflow: AutomationWorkflowTemplate, request_data: dict) -> tuple[bool, str | None, dict | None]:
    inventory_from_request = request_data.get('inventory_id')
    selected_inventory = None

    if str(inventory_from_request or '').isdigit():
        selected_inventory = AutomationInventory.objects.filter(id=int(inventory_from_request)).first()
        if selected_inventory is None:
            return False, f'Inventory {inventory_from_request} not found', None
    elif getattr(workflow, 'default_inventory_id', None):
        selected_inventory = getattr(workflow, 'default_inventory', None)
        if selected_inventory is None:
            selected_inventory = AutomationInventory.objects.filter(id=workflow.default_inventory_id).first()
        if selected_inventory is None:
            return False, 'Workflow default inventory not found', None

    # inventory 是必选的，未配置或已被删除时拒绝启动
    if selected_inventory is None:
        return False, 'Workflow 未配置 Inventory，无法启动', None

    if selected_inventory is not None and not selected_inventory.enabled:
        return False, f'Inventory [{selected_inventory.name or selected_inventory.id}] is disabled', None

    host_ids = []
    group_ids = []
    inventory_id = None
    inventory_name = ''
    use_global_scope = False
    if selected_inventory is not None:
        use_global_scope = True
        inventory_id = selected_inventory.id
        inventory_name = selected_inventory.name or ''
        host_ids = [int(item) for item in (selected_inventory.selected_host_ids or []) if str(item).isdigit()]
        group_ids = [int(item) for item in (selected_inventory.selected_group_ids or []) if str(item).isdigit()]

    limit_locked = 'limit' in request_data or bool(str(getattr(workflow, 'default_limit', '') or '').strip())
    if 'limit' in request_data:
        limit_text = str(request_data.get('limit') or '').strip()
    else:
        limit_text = str(getattr(workflow, 'default_limit', '') or '').strip()

    return True, None, {
        'use_global_scope': use_global_scope,
        'inventory_id': inventory_id,
        'inventory_name': inventory_name,
        'host_ids': host_ids,
        'group_ids': group_ids,
        'limit': limit_text,
        'limit_locked': limit_locked,
    }


def _precheck_workflow_runtime_scope(runtime_scope: dict) -> tuple[bool, str, dict]:
    use_global_scope = bool(runtime_scope.get('use_global_scope'))
    limit_text = str(runtime_scope.get('limit') or '').strip()

    if not use_global_scope:
        return False, 'inventory_missing', {
            'resolved_host_count': 0,
            'matched_hosts_preview': [],
            'matched_hosts_preview_total': 0,
            'effective_limit': limit_text,
            'message': 'Workflow 未配置 Inventory，无法启动',
        }

    host_ids = [int(item) for item in (runtime_scope.get('host_ids') or []) if str(item).isdigit()]
    group_ids = [int(item) for item in (runtime_scope.get('group_ids') or []) if str(item).isdigit()]

    existing_group_ids = set(HostGroup.objects.filter(id__in=group_ids).values_list('id', flat=True))
    missing_group_ids = sorted(set(group_ids) - existing_group_ids)
    if missing_group_ids:
        return False, 'inventory_invalid', {
            'resolved_host_count': 0,
            'matched_hosts_preview': [],
            'matched_hosts_preview_total': 0,
            'effective_limit': limit_text,
            'missing_group_ids': missing_group_ids,
            'message': f'执行范围包含已删除主机组: {", ".join(str(item) for item in missing_group_ids)}',
        }

    inventory_snapshot = build_inventory_snapshot(host_ids=host_ids, group_ids=group_ids)
    inventory_snapshot = _apply_limit_to_inventory_snapshot(inventory_snapshot, limit_text)
    hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
    resolved_host_count = len(hosts) if isinstance(hosts, list) else 0

    if resolved_host_count == 0:
        inventory_name = str(runtime_scope.get('inventory_name') or '-')
        return False, 'inventory_empty', {
            'resolved_host_count': 0,
            'matched_hosts_preview': [],
            'matched_hosts_preview_total': 0,
            'effective_limit': limit_text,
            'message': f'Inventory [{inventory_name}] 当前无匹配主机',
        }

    hosts_list = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
    offline_count = sum(1 for h in hosts_list if isinstance(h, dict) and not h.get('agent_online'))

    if offline_count > 0:
        return False, 'has_offline_hosts', {
            'resolved_host_count': resolved_host_count,
            'offline_hosts_count': offline_count,
            'matched_hosts_preview': _build_limit_matched_hosts_preview(inventory_snapshot),
            'matched_hosts_preview_total': resolved_host_count,
            'effective_limit': limit_text,
            'message': f'有 {offline_count} 台主机 Agent 离线，请确保所有目标主机在线后再执行',
        }

    return True, 'ok', {
        'resolved_host_count': resolved_host_count,
        'offline_hosts_count': 0,
        'matched_hosts_preview': _build_limit_matched_hosts_preview(inventory_snapshot),
        'matched_hosts_preview_total': resolved_host_count,
        'effective_limit': limit_text,
        'message': f'预检通过，可匹配主机 {resolved_host_count} 台',
    }


def _validate_workflow_task_nodes(workflow_nodes: list[dict]) -> tuple[bool, str | None]:
    task_ids = set()
    for node in workflow_nodes:
        if not isinstance(node, dict):
            continue
        if str(node.get('node_type') or '').strip().lower() != 'task':
            continue
        task_id_text = str(node.get('task_id') or '').strip()
        if not task_id_text.isdigit():
            continue
        task_ids.add(int(task_id_text))

    if not task_ids:
        return True, None

    task_map = {
        int(getattr(task, 'pk')): task
        for task in AutomationTask.objects.filter(id__in=list(task_ids)).select_related('playbook_template', 'shell_script_template', 'inventory')
        if getattr(task, 'pk', None) is not None
    }

    for node in workflow_nodes:
        if not isinstance(node, dict):
            continue
        if str(node.get('node_type') or '').strip().lower() != 'task':
            continue
        task_id_text = str(node.get('task_id') or '').strip()
        if not task_id_text.isdigit():
            continue

        task_id = int(task_id_text)
        node_name = str(node.get('name') or node.get('key') or f'Task-{task_id}').strip()
        task = task_map.get(task_id)
        if task is None:
            return False, f'节点 [{node_name}] 引用的任务不存在 (task_id={task_id})'
        if not task.enabled:
            return False, f'节点 [{node_name}] 引用的任务已禁用: {task.name or task_id}'
        if _resolve_task_template(task) is None:
            return False, f'节点 [{node_name}] 引用任务缺少模板: {task.name or task_id}'
        task_inventory = task.inventory
        if task_inventory is not None and not task_inventory.enabled:
            return False, f'节点 [{node_name}] 引用任务的 Inventory 已禁用: {task_inventory.name or "-"}'

    return True, None


def _dispatch_workflow_task_job(run: AutomationWorkflowRun, node_result: dict) -> tuple[bool, str | None, int | None, dict | None]:
    task_id = node_result.get('task_id')
    if task_id is None or not str(task_id).isdigit():
        return False, 'Task node missing valid task_id', None, None

    task = AutomationTask.objects.filter(id=int(task_id)).select_related('playbook_template', 'shell_script_template', 'inventory').first()
    task_template = _resolve_task_template(task) if task is not None else None
    if task is None or task_template is None:
        return False, f'Task {task_id} not found or template missing', None, None

    # 已创建的 workflow run 视为快照语义：
    # 运行中节点派发不再受任务/Inventory 后续启用状态变更影响。

    runtime_scope = _extract_workflow_runtime_scope(run)
    if runtime_scope.get('use_global_scope'):
        default_host_ids = runtime_scope.get('host_ids') or []
        default_group_ids = runtime_scope.get('group_ids') or []
    else:
        default_host_ids = task.selected_host_ids
        default_group_ids = task.selected_group_ids
        if task.inventory_id and task.inventory is not None:
            default_host_ids = task.inventory.selected_host_ids
            default_group_ids = task.inventory.selected_group_ids

    host_ids = [int(item) for item in (default_host_ids or []) if str(item).isdigit()]
    group_ids = [int(item) for item in (default_group_ids or []) if str(item).isdigit()]
    if runtime_scope.get('limit_locked'):
        effective_limit = str(runtime_scope.get('limit') or '').strip()
    else:
        effective_limit = task.default_limit or ''

    inventory_snapshot = build_inventory_snapshot(host_ids=host_ids, group_ids=group_ids)
    inventory_snapshot = _apply_limit_to_inventory_snapshot(inventory_snapshot, effective_limit)
    hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
    if not isinstance(hosts, list) or len(hosts) == 0:
        if runtime_scope.get('use_global_scope'):
            scope_name = str(runtime_scope.get('inventory_name') or '-')
            return False, f'Workflow scope inventory [{scope_name}] resolved empty host scope', None, None
        return False, f'Task {task_id} resolved empty host scope', None, None

    node_name = str(node_result.get('node_name') or node_result.get('node_key') or f'Task-{task.id}')
    is_shell_task = task.shell_script_template_id is not None
    job = AutomationExecutionJob.objects.create(
        task=task,
        status=AutomationExecutionJob.Status.PENDING,
        trigger_type=AutomationExecutionJob.TriggerType.MANUAL,
        inventory_snapshot=inventory_snapshot,
        task_name_snapshot=task.name or '',
        template_name_snapshot=task_template.name or '',
        template_content_snapshot=task_template.content or '',
        extra_vars=run.extra_vars if (not is_shell_task and isinstance(run.extra_vars, dict)) else {},
        shell_parameters=task.shell_parameters if is_shell_task else '',
        shell_env_vars=task.env_vars if is_shell_task else {},
        limit=effective_limit,
        requested_user_id=run.requested_user_id,
        requested_username=run.requested_username or '',
        result_summary={'message': f'Workflow node {node_name} dispatched'},
        # 权限提升配置快照
        become_enabled_snapshot=task.become_enabled,
        become_method_snapshot=task.become_method,
        become_user_snapshot=task.become_user,
    )

    try:
        # 非 Celery：改为本地后台线程执行，避免阻塞当前 HTTP 请求。
        run_job_in_background(int(job.id))
    except Exception as exc:
        job.status = AutomationExecutionJob.Status.FAILED
        job.result_summary = {'message': f'Failed to start local runner: {str(exc)}'}
        job.save(update_fields=['status', 'result_summary'])
        return False, f'Failed to start local runner: {str(exc)}', None, None

    return True, None, job.id, {
        'task_name_snapshot': task.name or '',
        'job_template_id': int(task_template.id),
        'template_name_snapshot': task_template.name or '',
    }


def _dispatch_workflow_child_run(run: AutomationWorkflowRun, node_result: dict) -> tuple[bool, str | None, int | None, dict | None]:
    workflow_id = node_result.get('workflow_id')
    if workflow_id is None or not str(workflow_id).isdigit():
        return False, 'Workflow node missing valid workflow_id', None, None

    child_workflow = AutomationWorkflowTemplate.objects.filter(id=int(workflow_id)).first()
    if child_workflow is None:
        return False, f'Workflow {workflow_id} not found', None, None
    if not child_workflow.enabled:
        return False, f'Workflow {workflow_id} is disabled', None, None

    target_workflow_id = int(workflow_id)
    ancestor_template_ids = _read_workflow_ancestor_template_ids(run)
    # AWX 风格：允许模板保存时自引用，但在运行时按祖先链阻断递归派生。
    if target_workflow_id in set(ancestor_template_ids):
        chain_text = _format_workflow_spawn_chain_display(target_workflow_id, ancestor_template_ids)
        return False, f'Recursion detected (spawn order, most recent first): {chain_text}', None, None

    child_workflow_nodes_snapshot = child_workflow.nodes if isinstance(child_workflow.nodes, list) else []
    if len(child_workflow_nodes_snapshot) == 0:
        return False, f'Workflow {workflow_id} plan is empty', None, None

    # 在派发前检测该工作流内部是否存在循环引用
    is_cycle, cycle_error_msg = check_workflow_cycle_at_runtime(target_workflow_id, child_workflow_nodes_snapshot)
    if is_cycle:
        return False, cycle_error_msg, None, None

    child_ancestor_template_ids = list(ancestor_template_ids)
    child_ancestor_template_ids.append(target_workflow_id)

    child_run = AutomationWorkflowRun.objects.create(
        workflow=child_workflow,
        status=AutomationWorkflowRun.Status.RUNNING,
        trigger_type=AutomationWorkflowRun.TriggerType.MANUAL,
        workflow_name_snapshot=child_workflow.name or '',
        workflow_code_snapshot='',
        planned_node_keys=[str(item.get('key') or '').strip() for item in child_workflow_nodes_snapshot if str(item.get('key') or '').strip()],
        node_results=[],
        extra_vars=run.extra_vars if isinstance(run.extra_vars, dict) else {},
        result_summary={'message': 'Workflow run created from workflow node'},
        requested_user_id=run.requested_user_id,
        requested_username=run.requested_username or '',
        start_time=timezone.now(),
    )

    child_node_results = _build_initial_node_results_from_nodes(child_workflow_nodes_snapshot)
    child_workflow_edges_snapshot = child_workflow.edges if isinstance(child_workflow.edges, list) else []
    child_run.node_results = child_node_results
    child_run.result_summary = {
        'message': 'Workflow run created from workflow node',
        'queued_job_count': 0,
        'queued_job_ids': [],
        'parent_run_id': run.id,
        'runtime_scope': _extract_workflow_runtime_scope(run),
        # 子 run 继承并扩展祖先模板链，供后续嵌套节点继续做递归检测。
        'workflow_ancestor_template_ids': child_ancestor_template_ids,
        'workflow_nodes_snapshot': json.loads(json.dumps(child_workflow_nodes_snapshot, ensure_ascii=False)),
        'workflow_edges_snapshot': json.loads(json.dumps(child_workflow_edges_snapshot, ensure_ascii=False)),
    }
    child_run.save(update_fields=['node_results', 'result_summary'])
    _refresh_workflow_run_progress(child_run)

    return True, None, child_run.id, {
        'workflow_name_snapshot': child_workflow.name or '',
    }


def _refresh_workflow_run_progress(run: AutomationWorkflowRun):
    if run.status in (
        AutomationWorkflowRun.Status.SUCCESS,
    ):
        return

    node_results = [item for item in (run.node_results or []) if isinstance(item, dict)]
    if len(node_results) == 0:
        return

    job_ids = [int(item['job_id']) for item in node_results if str(item.get('job_id', '')).isdigit()]
    child_run_ids = [int(item['child_run_id']) for item in node_results if str(item.get('child_run_id', '')).isdigit()]
    job_status_map = {}
    child_run_status_map = {}
    if job_ids:
        rows = AutomationExecutionJob.objects.filter(id__in=list(set(job_ids))).values('id', 'status')
        job_status_map = {int(row['id']): str(row.get('status') or '').lower() for row in rows}
    if child_run_ids:
        rows = AutomationWorkflowRun.objects.filter(id__in=list(set(child_run_ids))).values('id', 'status')
        child_run_status_map = {int(row['id']): str(row.get('status') or '').lower() for row in rows}

    for item in node_results:
        job_id = item.get('job_id')
        if not str(job_id).isdigit():
            continue
        job_status = job_status_map.get(int(job_id), '')
        if not job_status:
            continue
        if job_status == 'pending':
            item['status'] = 'queued'
        elif job_status == 'running':
            item['status'] = 'running'
        elif job_status == 'success':
            item['status'] = 'success'
        elif job_status == 'failed':
            item['status'] = 'failed'
        elif job_status == 'cancelled':
            item['status'] = 'cancelled'

    for item in node_results:
        child_run_id = item.get('child_run_id')
        if not str(child_run_id).isdigit():
            continue
        child_status = child_run_status_map.get(int(child_run_id), '')
        if not child_status:
            continue
        if child_status in {'pending', 'running'}:
            item['status'] = 'running'
        elif child_status == 'success':
            item['status'] = 'success'
        elif child_status == 'failed':
            item['status'] = 'failed'

    result_summary = run.result_summary if isinstance(run.result_summary, dict) else {}
    is_cancelled_by_user = bool(result_summary.get('cancelled'))
    snapshot_nodes = result_summary.get('workflow_nodes_snapshot', [])
    workflow_nodes = snapshot_nodes if isinstance(snapshot_nodes, list) else []
    if len(workflow_nodes) == 0:
        workflow = getattr(run, 'workflow', None)
        workflow_nodes = workflow.nodes if workflow is not None and isinstance(workflow.nodes, list) else []

    node_order_map = {}
    for node in workflow_nodes:
        if not isinstance(node, dict):
            continue
        node_key = str(node.get('key') or '').strip()
        if not node_key:
            continue
        node_order_map[node_key] = {
            'y': float(node.get('y') or 0),
            'x': float(node.get('x') or 0),
        }

    snapshot_edges = result_summary.get('workflow_edges_snapshot', [])
    workflow_edges = snapshot_edges if isinstance(snapshot_edges, list) else []
    if len(workflow_edges) == 0:
        workflow = getattr(run, 'workflow', None)
        workflow_edges = workflow.edges if workflow is not None and isinstance(workflow.edges, list) else []

    node_result_map = {}
    for item in node_results:
        node_key = str(item.get('node_key') or '').strip()
        if node_key:
            node_result_map[node_key] = item
            if node_key not in node_order_map:
                node_order_map[node_key] = {'y': 0.0, 'x': 0.0}

    incoming_edge_map = {}
    outgoing_edge_map = {key: [] for key in node_order_map.keys()}
    incoming_count_map = {key: 0 for key in node_order_map.keys()}
    for edge in workflow_edges:
        if not isinstance(edge, dict):
            continue
        source_key = str(edge.get('source_key') or '').strip()
        target_key = str(edge.get('target_key') or '').strip()
        if not source_key or not target_key:
            continue
        incoming_edge_map.setdefault(target_key, []).append({
            'source_key': source_key,
            'condition': str(edge.get('condition') or 'success').strip().lower() or 'success',
        })
        if source_key in outgoing_edge_map and target_key in incoming_count_map:
            outgoing_edge_map[source_key].append(target_key)
            incoming_count_map[target_key] = int(incoming_count_map.get(target_key, 0)) + 1

    def _node_sort_tuple(node_key: str, node_name: str = ''):
        info = node_order_map.get(node_key, {})
        return (
            float(info.get('y', 0)),
            float(info.get('x', 0)),
            str(node_name or ''),
            str(node_key or ''),
        )

    # 对 snapshot 图做拓扑分层：只推进当前最浅未完成层，避免越层执行。
    node_depth_map = {key: 1 for key in node_order_map.keys()}
    depth_queue = sorted(
        [key for key, count in incoming_count_map.items() if int(count) == 0],
        key=lambda item: _node_sort_tuple(item),
    )

    while depth_queue:
        current_key = depth_queue.pop(0)
        current_depth = int(node_depth_map.get(current_key, 1))
        for child_key in outgoing_edge_map.get(current_key, []):
            node_depth_map[child_key] = max(int(node_depth_map.get(child_key, 1)), current_depth + 1)
            incoming_count_map[child_key] = int(incoming_count_map.get(child_key, 0)) - 1
            if int(incoming_count_map.get(child_key, 0)) == 0:
                depth_queue.append(child_key)
        depth_queue.sort(key=lambda item: _node_sort_tuple(item))

    terminal_statuses = {'success', 'failed', 'cancelled', 'skipped'}

    def _edge_condition_matched(condition: str, parent_status: str) -> bool:
        if condition == 'always':
            return parent_status in terminal_statuses
        if condition == 'failure':
            return parent_status in {'failed', 'cancelled'}
        return parent_status == 'success'

    def _evaluate_node_state(item: dict) -> str:
        node_key = str(item.get('node_key') or '').strip()
        convergence = str(item.get('convergence') or 'any').strip().lower() or 'any'
        incoming_edges = incoming_edge_map.get(node_key, [])
        if len(incoming_edges) == 0:
            return 'ready'

        all_edges_matched = True
        matched_any_edge = False
        all_parents_finished = True

        for edge in incoming_edges:
            source_key = str(edge.get('source_key') or '').strip()
            parent = node_result_map.get(source_key)
            if parent is None:
                all_parents_finished = False
                all_edges_matched = False
                continue

            parent_status = str(parent.get('status') or '').lower()
            if parent_status not in terminal_statuses:
                all_parents_finished = False
                all_edges_matched = False

            matched = _edge_condition_matched(str(edge.get('condition') or 'success'), parent_status)
            if matched:
                matched_any_edge = True
            else:
                all_edges_matched = False

        if convergence == 'all':
            if all_parents_finished and all_edges_matched:
                return 'ready'
            if all_parents_finished and not all_edges_matched:
                return 'skipped'
            return 'waiting'

        if matched_any_edge:
            return 'ready'
        if all_parents_finished:
            return 'skipped'
        return 'waiting'

    level_unfinished_nodes = []
    for item in node_results:
        node_key = str(item.get('node_key') or '').strip()
        status = str(item.get('status') or '').lower()
        if not node_key:
            continue
        if status in {'waiting', 'pending', 'queued', 'running'}:
            level_unfinished_nodes.append(item)

    if is_cancelled_by_user:
        for item in node_results:
            status = str(item.get('status') or '').lower()
            if status not in {'waiting', 'pending', 'queued', 'running'}:
                continue

            if str(item.get('job_id', '')).isdigit():
                item['status'] = 'cancelled'
            else:
                item['status'] = 'skipped'
            item['message'] = 'Workflow run cancelled by user'

        level_unfinished_nodes = []

    if level_unfinished_nodes:
        active_depth = min(
            int(node_depth_map.get(str(item.get('node_key') or '').strip(), 1))
            for item in level_unfinished_nodes
        )
        current_level_nodes = [
            item
            for item in level_unfinished_nodes
            if int(node_depth_map.get(str(item.get('node_key') or '').strip(), 1)) == active_depth
        ]

        current_level_running = [
            item for item in current_level_nodes if str(item.get('status') or '').lower() in {'queued', 'running'}
        ]
        current_level_waiting = [
            item for item in current_level_nodes if str(item.get('status') or '').lower() in {'waiting', 'pending'}
        ]
        node_eval_map = {}
        level_ready_nodes = []
        for item in current_level_waiting:
            node_key = str(item.get('node_key') or '').strip()
            eval_state = _evaluate_node_state(item)
            if node_key:
                node_eval_map[node_key] = eval_state
            if eval_state == 'ready':
                level_ready_nodes.append(item)

        level_ready_nodes.sort(
            key=lambda item: _node_sort_tuple(
                str(item.get('node_key') or '').strip(),
                str(item.get('node_name') or ''),
            )
        )

        # 串行调度：同层每轮最多派发 1 个 ready 节点，避免并行运行。
        dispatch_node = None
        if not current_level_running and level_ready_nodes:
            dispatch_node = level_ready_nodes[0]

        dispatch_key = str(dispatch_node.get('node_key') or '').strip() if isinstance(dispatch_node, dict) else ''

        for waiting_node in current_level_waiting:
            node_key = str(waiting_node.get('node_key') or '').strip()
            if dispatch_key and node_key == dispatch_key:
                continue
            eval_state = node_eval_map.get(node_key, 'waiting')
            if eval_state == 'skipped':
                waiting_node['status'] = 'skipped'
                waiting_node['message'] = '条件不满足，节点已跳过'
                continue
            if str(waiting_node.get('status') or '').lower() == 'waiting' and eval_state == 'ready':
                waiting_node['status'] = 'pending'

        if dispatch_node:
            queued_ids = result_summary.get('queued_job_ids', [])
            queued_ids = queued_ids if isinstance(queued_ids, list) else []

            node_type = str(dispatch_node.get('node_type') or '').lower()
            if node_type == 'workflow':
                ok, error_msg, child_run_id, dispatch_meta = _dispatch_workflow_child_run(run, dispatch_node)
                if ok and child_run_id is not None:
                    dispatch_node['status'] = 'queued'
                    dispatch_node['child_run_id'] = child_run_id
                    if isinstance(dispatch_meta, dict):
                        dispatch_node['workflow_name_snapshot'] = str(dispatch_meta.get('workflow_name_snapshot') or '').strip()
                else:
                    dispatch_node['status'] = 'failed'
                    dispatch_node['message'] = error_msg or 'Workflow node dispatch failed'
            else:
                ok, error_msg, job_id, dispatch_meta = _dispatch_workflow_task_job(run, dispatch_node)
                if ok and job_id is not None:
                    dispatch_node['status'] = 'queued'
                    dispatch_node['job_id'] = job_id
                    if isinstance(dispatch_meta, dict):
                        dispatch_node['task_name_snapshot'] = str(dispatch_meta.get('task_name_snapshot') or '').strip()
                        dispatch_node['job_template_id'] = (
                            int(dispatch_meta['job_template_id'])
                            if str(dispatch_meta.get('job_template_id', '')).isdigit()
                            else None
                        )
                        dispatch_node['template_name_snapshot'] = str(dispatch_meta.get('template_name_snapshot') or '').strip()
                    queued_ids.append(job_id)
                else:
                    dispatch_node['status'] = 'failed'
                    dispatch_node['message'] = error_msg or 'Node dispatch failed'

            result_summary['queued_job_ids'] = queued_ids
            result_summary['queued_job_count'] = len(queued_ids)
            run.result_summary = result_summary

    # AWX 风格聚合：统一交给共享 helper，避免 views / serializer 各写一份。
    run.status = get_workflow_runtime_status(node_results, workflow_edges, fallback_status=run.status)

    now = timezone.now()
    run.node_results = node_results
    if run.status in WORKFLOW_RUNTIME_FINAL_STATUSES:
        # 终态只在首次收敛时写入 end_time；后续刷新不可持续覆盖，
        # 否则列表/详情每次查询都会把“耗时”越算越大。
        if run.end_time is None:
            run.end_time = now

        # duration 优先保持已落库值；仅在缺失时做一次回填。
        if run.duration_seconds is None and run.start_time and run.end_time:
            run.duration_seconds = max((run.end_time - run.start_time).total_seconds(), 0.0)
    else:
        run.end_time = None
        run.duration_seconds = None

    run.save(update_fields=['status', 'node_results', 'result_summary', 'end_time', 'duration_seconds'])


