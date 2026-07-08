import json
import os
import re
import fnmatch
from urllib.parse import quote

from django.http import HttpResponse
from django.utils.dateparse import parse_datetime
from django.utils import timezone
from django.db.models import Q
from django_filters.rest_framework import DjangoFilterBackend
from rest_framework.decorators import action
from rest_framework.filters import OrderingFilter, SearchFilter
from rest_framework.mixins import CreateModelMixin, DestroyModelMixin, ListModelMixin, RetrieveModelMixin, UpdateModelMixin
from rest_framework import serializers
from rest_framework.viewsets import GenericViewSet

from djadmin.utils import CustomPagination, Response_200, Response_error_str
from menu.permisssion import CustomMenuPermission
from user.utils import getCurrentUser
from assets.models import Host, HostGroup

from .models import (
    PlaybookTemplate,
    AutomationTask,
    AutomationInventory,
    AnsibleExecutionJob,
    AnsibleExecutionTarget,
    AutomationWorkflowTemplate,
    AutomationWorkflowRun,
)
from .serializer import (
    PlaybookTemplateSerializer,
    AutomationTaskSerializer,
    AutomationInventorySerializer,
    AnsibleExecutionJobSerializer,
    AnsibleExecutionTargetSerializer,
    AutomationWorkflowTemplateSerializer,
    AutomationWorkflowRunSerializer,
    validate_playbook_content_or_raise,
    check_workflow_cycle_at_runtime,
)
from .executor import build_inventory_snapshot
from .tasks import execute_ansible_job_task
from .workflow_runtime import WORKFLOW_RUNTIME_FINAL_STATUSES, get_workflow_runtime_status


def _build_initial_node_results_from_nodes(nodes: list[dict]) -> list[dict]:
    task_ids = {
        int(node.get('task_id'))
        for node in nodes
        if isinstance(node, dict)
        and str(node.get('task_id', '')).isdigit()
        and str(node.get('node_type') or '').strip().lower() == 'task'
    }
    workflow_ids = {
        int(node.get('workflow_id'))
        for node in nodes
        if isinstance(node, dict)
        and str(node.get('workflow_id', '')).isdigit()
        and str(node.get('node_type') or '').strip().lower() == 'workflow'
    }

    task_snapshot_map = {}
    if task_ids:
        rows = AutomationTask.objects.filter(id__in=list(task_ids)).select_related('template').values(
            'id', 'name', 'template_id', 'template__name'
        )
        task_snapshot_map = {
            int(row['id']): {
                'task_name_snapshot': str(row.get('name') or '').strip(),
                'template_id_snapshot': int(row['template_id']) if str(row.get('template_id', '')).isdigit() else None,
                'template_name_snapshot': str(row.get('template__name') or '').strip(),
            }
            for row in rows
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
        task_id_value = int(node.get('task_id')) if str(node.get('task_id', '')).isdigit() else None
        workflow_id_value = int(node.get('workflow_id')) if str(node.get('workflow_id', '')).isdigit() else None
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
            'template_id_snapshot': task_snapshot.get('template_id_snapshot'),
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
    pattern = str(token or '').strip().lower()
    if not pattern:
        return False

    host_id_text = str(host_item.get('host_id') or '')
    host_name = str(host_item.get('host_name') or '').lower()
    host_ip = str(host_item.get('host_ip') or '').lower()
    group_name = str(host_item.get('group_name') or '').lower()
    group_path = str(host_item.get('group_path') or '').lower()
    if fnmatch.fnmatch(host_id_text, pattern):
        return True
    if fnmatch.fnmatch(host_name, pattern):
        return True
    if fnmatch.fnmatch(host_ip, pattern):
        return True
    if fnmatch.fnmatch(group_name, pattern):
        return True
    if fnmatch.fnmatch(group_path, pattern):
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


def _dispatch_workflow_task_job(run: AutomationWorkflowRun, node_result: dict) -> tuple[bool, str | None, int | None, dict | None]:
    task_id = node_result.get('task_id')
    if task_id is None or not str(task_id).isdigit():
        return False, 'Task node missing valid task_id', None, None

    task = AutomationTask.objects.filter(id=int(task_id)).select_related('template', 'inventory').first()
    if task is None or task.template is None:
        return False, f'Task {task_id} not found or template missing', None, None

    default_host_ids = task.selected_host_ids
    default_group_ids = task.selected_group_ids
    if task.inventory_id and task.inventory is not None:
        default_host_ids = task.inventory.selected_host_ids
        default_group_ids = task.inventory.selected_group_ids

    host_ids = [int(item) for item in (default_host_ids or []) if str(item).isdigit()]
    group_ids = [int(item) for item in (default_group_ids or []) if str(item).isdigit()]
    effective_limit = task.default_limit or ''

    inventory_snapshot = build_inventory_snapshot(host_ids=host_ids, group_ids=group_ids)
    inventory_snapshot = _apply_limit_to_inventory_snapshot(inventory_snapshot, effective_limit)
    hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
    if not isinstance(hosts, list) or len(hosts) == 0:
        return False, f'Task {task_id} resolved empty host scope', None, None

    node_name = str(node_result.get('node_name') or node_result.get('node_key') or f'Task-{task.id}')
    job = AnsibleExecutionJob.objects.create(
        template=task.template,
        task=task,
        status=AnsibleExecutionJob.Status.PENDING,
        trigger_type=AnsibleExecutionJob.TriggerType.MANUAL,
        inventory_snapshot=inventory_snapshot,
        task_name_snapshot=task.name or '',
        template_name_snapshot=task.template.name or '',
        template_content_snapshot=task.template.content or '',
        extra_vars=run.extra_vars if isinstance(run.extra_vars, dict) else {},
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
        execute_ansible_job_task.delay(job.id)
    except Exception as exc:
        job.status = AnsibleExecutionJob.Status.FAILED
        job.result_summary = {'message': f'Failed to enqueue job: {str(exc)}'}
        job.save(update_fields=['status', 'result_summary'])
        return False, f'Failed to enqueue task job: {str(exc)}', None, None

    return True, None, job.id, {
        'task_name_snapshot': task.name or '',
        'template_id_snapshot': task.template_id,
        'template_name_snapshot': task.template.name or '',
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
        rows = AnsibleExecutionJob.objects.filter(id__in=list(set(job_ids))).values('id', 'status')
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
                        dispatch_node['template_id_snapshot'] = (
                            int(dispatch_meta['template_id_snapshot'])
                            if str(dispatch_meta.get('template_id_snapshot', '')).isdigit()
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
        run.end_time = now
        run.duration_seconds = (now - run.start_time).total_seconds() if run.start_time else None
    else:
        run.end_time = None
        run.duration_seconds = None

    run.save(update_fields=['status', 'node_results', 'result_summary', 'end_time', 'duration_seconds'])


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
            validate_playbook_content_or_raise(content)
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
            validate_playbook_content_or_raise(content)
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

        job = AnsibleExecutionJob.objects.create(
            template=template,
            status=AnsibleExecutionJob.Status.PENDING,
            trigger_type=AnsibleExecutionJob.TriggerType.MANUAL,
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
            execute_ansible_job_task.delay(job.id)
        except Exception as exc:
            job.status = AnsibleExecutionJob.Status.FAILED
            job.result_summary = {'message': f'Failed to enqueue job: {str(exc)}'}
            job.save(update_fields=['status', 'result_summary'])
            return Response_error_str(f'Job enqueue failed: {str(exc)}', code=400)

        serializer = AnsibleExecutionJobSerializer(job)
        return Response_200(data=serializer.data)


class AutomationTaskManage(GenericViewSet, CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin):
    queryset = AutomationTask.objects.select_related('template', 'inventory').all()
    serializer_class = AutomationTaskSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'code', 'template__name', 'inventory__name', 'remark']
    ordering_fields = ['name', 'code', 'create_time', 'update_time']
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

        if not task.template:
            return Response_200(data={
                'ok': False,
                'status': 'template_missing',
                'message': '任务模板不存在',
                'resolved_host_count': 0,
            })

        if task.inventory_id and task.inventory is not None and not task.inventory.enabled:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_disabled',
                'message': 'Inventory 已禁用，无法执行',
                'resolved_host_count': 0,
            })

        default_host_ids = task.selected_host_ids
        default_group_ids = task.selected_group_ids
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
        if not task.enabled:
            return Response_error_str('Task is disabled', code=400)
        if not task.template:
            return Response_error_str('Template is missing', code=400)
        if task.inventory_id and task.inventory is not None and not task.inventory.enabled:
            return Response_error_str('Inventory is disabled', code=400)

        user_info = getCurrentUser(request)
        default_host_ids = task.selected_host_ids
        default_group_ids = task.selected_group_ids
        if task.inventory_id and task.inventory is not None:
            default_host_ids = task.inventory.selected_host_ids
            default_group_ids = task.inventory.selected_group_ids

        limit_text = str(request.data.get('limit', task.default_limit or '')).strip()

        host_ids_raw = request.data.get('host_ids', default_host_ids)
        group_ids_raw = request.data.get('group_ids', default_group_ids)
        extra_vars_raw = request.data.get('extra_vars', task.env_vars)

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        group_ids = group_ids_raw if isinstance(group_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        group_ids = [int(item) for item in group_ids if str(item).isdigit()]
        extra_vars = extra_vars_raw if isinstance(extra_vars_raw, dict) else {}

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

        job = AnsibleExecutionJob.objects.create(
            template=task.template,
            task=task,
            status=AnsibleExecutionJob.Status.PENDING,
            trigger_type=AnsibleExecutionJob.TriggerType.MANUAL,
            inventory_snapshot=inventory_snapshot,
            task_name_snapshot=task.name or '',
            template_name_snapshot=task.template.name or '',
            template_content_snapshot=task.template.content or '',
            extra_vars=extra_vars,
            limit=limit_text,
            requested_user_id=user_info.get('user_id'),
            requested_username=user_info.get('username', ''),
            result_summary={'message': 'Job created and waiting for runner'},
            # 权限提升配置快照
            become_enabled_snapshot=task.become_enabled,
            become_method_snapshot=task.become_method,
            become_user_snapshot=task.become_user,
        )

        try:
            execute_ansible_job_task.delay(job.id)
        except Exception as exc:
            job.status = AnsibleExecutionJob.Status.FAILED
            job.result_summary = {'message': f'Failed to enqueue job: {str(exc)}'}
            job.save(update_fields=['status', 'result_summary'])
            return Response_error_str(f'Job enqueue failed: {str(exc)}', code=400)

        serializer = AnsibleExecutionJobSerializer(job)
        return Response_200(data=serializer.data)


class AutomationInventoryManage(GenericViewSet, CreateModelMixin, UpdateModelMixin, RetrieveModelMixin, ListModelMixin, DestroyModelMixin):
    queryset = AutomationInventory.objects.all()
    serializer_class = AutomationInventorySerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'remark']
    ordering_fields = ['name', 'create_time', 'update_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:inventory:view',
        'retrieve': 'automation:inventory:view',
        'create': 'automation:inventory:create',
        'destroy': 'automation:inventory:delete',
        'partial_update': 'automation:inventory:update',
        'perform_update': 'automation:inventory:update',
        'precheck_limit': 'automation:inventory:view',
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
        instance = self.get_object()
        deleted_id = instance.id
        self.perform_destroy(instance)
        return Response_200(data={'id': deleted_id})

    @action(detail=True, methods=['post'], url_path='precheck-limit')
    def precheck_limit(self, request, id=None):
        inventory = self.get_object()

        if not inventory.enabled:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_disabled',
                'message': 'Inventory 已禁用，无法执行预检',
                'resolved_host_count': 0,
                'effective_limit': '',
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
            })

        limit_text = str(request.data.get('limit', '')).strip()
        host_ids_raw = request.data.get('host_ids', inventory.selected_host_ids)
        group_ids_raw = request.data.get('group_ids', inventory.selected_group_ids)

        host_ids = host_ids_raw if isinstance(host_ids_raw, list) else []
        group_ids = group_ids_raw if isinstance(group_ids_raw, list) else []
        host_ids = [int(item) for item in host_ids if str(item).isdigit()]
        group_ids = [int(item) for item in group_ids if str(item).isdigit()]

        existing_group_ids = set(HostGroup.objects.filter(id__in=group_ids).values_list('id', flat=True))
        missing_group_ids = sorted(set(group_ids) - existing_group_ids)
        if missing_group_ids:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_invalid',
                'message': f'执行范围包含已删除主机组: {", ".join(str(item) for item in missing_group_ids)}',
                'resolved_host_count': 0,
                'effective_limit': limit_text,
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
                'missing_group_ids': missing_group_ids,
            })

        inventory_snapshot = build_inventory_snapshot(host_ids=host_ids, group_ids=group_ids)
        inventory_snapshot = _apply_limit_to_inventory_snapshot(inventory_snapshot, limit_text)
        hosts = inventory_snapshot.get('hosts', []) if isinstance(inventory_snapshot, dict) else []
        resolved_host_count = len(hosts) if isinstance(hosts, list) else 0

        if resolved_host_count == 0:
            return Response_200(data={
                'ok': False,
                'status': 'inventory_empty',
                'message': f'Inventory [{inventory.name or "-"}] 当前无匹配主机',
                'resolved_host_count': 0,
                'effective_limit': limit_text,
                'matched_hosts_preview': [],
                'matched_hosts_preview_total': 0,
            })

        return Response_200(data={
            'ok': True,
            'status': 'ok',
            'message': f'预检通过，可匹配主机 {resolved_host_count} 台',
            'resolved_host_count': resolved_host_count,
            'effective_limit': limit_text,
            'matched_hosts_preview': _build_limit_matched_hosts_preview(inventory_snapshot),
            'matched_hosts_preview_total': resolved_host_count,
        })


class AnsibleExecutionJobManage(GenericViewSet, RetrieveModelMixin, ListModelMixin):
    queryset = AnsibleExecutionJob.objects.select_related('template', 'task').all()
    serializer_class = AnsibleExecutionJobSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['job_id', 'requested_username', 'template__name', 'task__name', 'remark']
    ordering_fields = ['create_time', 'update_time', 'start_time', 'end_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:jobs:view',
        'retrieve': 'automation:jobs:view',
        'cancel': 'automation:jobs:cancel',
        'log': 'automation:jobs:view',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        status_value = self.request.query_params.get('status')  # type: ignore[union-attr]
        template_id = self.request.query_params.get('template_id')  # type: ignore[union-attr]
        task_id = self.request.query_params.get('task_id')  # type: ignore[union-attr]
        job_id = self.request.query_params.get('job_id')  # type: ignore[union-attr]
        keyword = self.request.query_params.get('keyword')  # type: ignore[union-attr]
        output_keyword = self.request.query_params.get('output_keyword')  # type: ignore[union-attr]
        start_time_from = self.request.query_params.get('start_time_from')  # type: ignore[union-attr]
        start_time_to = self.request.query_params.get('start_time_to')  # type: ignore[union-attr]

        if status_value:
            queryset = queryset.filter(status=status_value)
        if template_id:
            queryset = queryset.filter(template_id=template_id)
        if task_id:
            queryset = queryset.filter(task_id=task_id)
        if job_id:
            # 按执行记录 ID 精确查询
            if str(job_id).isdigit():
                queryset = queryset.filter(id=int(job_id))
        elif keyword:
            condition = (
                Q(job_id__icontains=keyword) |
                Q(requested_username__icontains=keyword) |
                Q(template__name__icontains=keyword) |
                Q(task__name__icontains=keyword)
            )
            if str(keyword).isdigit():
                condition = condition | Q(id=int(keyword))
            queryset = queryset.filter(condition)

        if output_keyword:
            queryset = queryset.filter(job_output__icontains=output_keyword)

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


class AutomationWorkflowTemplateManage(
    GenericViewSet,
    CreateModelMixin,
    UpdateModelMixin,
    RetrieveModelMixin,
    ListModelMixin,
    DestroyModelMixin,
):
    queryset = AutomationWorkflowTemplate.objects.all()
    serializer_class = AutomationWorkflowTemplateSerializer
    pagination_class = CustomPagination
    filter_backends = (OrderingFilter, DjangoFilterBackend, SearchFilter)
    search_fields = ['name', 'description', 'remark']
    ordering_fields = ['name', 'create_time', 'update_time']
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
        instance = self.get_object()
        deleted_id = instance.id
        self.perform_destroy(instance)
        return Response_200(data={'id': deleted_id})

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
    search_fields = ['workflow__name', 'requested_username', 'remark']
    ordering_fields = ['create_time', 'update_time', 'start_time', 'end_time']
    lookup_field = 'id'
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        'list': 'automation:workflow:view',
        'retrieve': 'automation:workflow:view',
        'cancel': 'automation:jobs:cancel',
    }

    def get_queryset(self):
        queryset = super().get_queryset()
        workflow_id = self.request.query_params.get('workflow_id')  # type: ignore[union-attr]
        status_value = self.request.query_params.get('status')  # type: ignore[union-attr]
        if workflow_id:
            queryset = queryset.filter(workflow_id=workflow_id)
        if status_value:
            queryset = queryset.filter(status=status_value)
        return queryset

    def list(self, request, *args, **kwargs):
        queryset = self.filter_queryset(self.get_queryset())
        page = self.paginate_queryset(queryset)
        run_items = list(page) if page is not None else list(queryset)
        for item in run_items:
            _refresh_workflow_run_progress(item)
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

                AnsibleExecutionTarget.objects.filter(job=job, status__in=[
                    AnsibleExecutionTarget.Status.PENDING,
                    AnsibleExecutionTarget.Status.RUNNING,
                ]).update(
                    status=AnsibleExecutionTarget.Status.SKIPPED,
                    end_time=now,
                    stderr='Cancelled by workflow run cancellation',
                )

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
