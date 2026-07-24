from typing import Any

from django.db import close_old_connections
from django.utils import timezone

from assets.models import Host, HostGroup

from .models import AutomationExecutionJob
from .executor_shell_script import execute_shell_script_job
from .agent_grpc_runner import execute_job_via_agent_grpc


def _build_group_descendants(group_ids: list[int]) -> set[int]:
    if not group_ids:
        return set()

    id_set = set(group_ids)
    queue = list(group_ids)

    while queue:
        current = queue.pop(0)
        children = HostGroup.objects.filter(parent_id=current).values_list('id', flat=True)
        for child_id in children:
            if child_id not in id_set:
                id_set.add(child_id)
                queue.append(child_id)

    return id_set


def _collect_hosts(host_ids: list[int], group_ids: list[int]) -> list[Host]:
    queryset = Host.objects.filter(ip__isnull=False).select_related('group').order_by('id')

    conditions = []
    if host_ids:
        conditions.append({'id__in': host_ids})

    group_id_set = _build_group_descendants(group_ids)
    if group_id_set:
        conditions.append({'group_id__in': list(group_id_set)})

    if not conditions:
        # Empty scope means "all hosts with IP" for automation task execution.
        return list(queryset)

    combined_ids = set()
    for condition in conditions:
        ids = queryset.filter(**condition).values_list('id', flat=True)
        combined_ids.update(ids)

    if not combined_ids:
        return []

    return list(queryset.filter(id__in=list(combined_ids)).order_by('id'))


def _build_group_path_map(group_ids: list[int]) -> dict[int, str]:
    normalized_ids = [int(item) for item in group_ids if isinstance(item, int)]
    if not normalized_ids:
        return {}

    group_rows = list(HostGroup.objects.all().values('id', 'name', 'parent_id'))
    group_lookup = {int(item['id']): item for item in group_rows if item.get('id') is not None}
    cache: dict[int, str] = {}

    def resolve_path(group_id: int) -> str:
        if group_id in cache:
            return cache[group_id]
        row = group_lookup.get(group_id)
        if not row:
            cache[group_id] = ''
            return ''

        name = str(row.get('name') or '').strip()
        parent_id_raw = row.get('parent_id')
        parent_id = int(parent_id_raw) if isinstance(parent_id_raw, int) else None
        if parent_id and parent_id != group_id:
            parent_path = resolve_path(parent_id)
            cache[group_id] = f'{parent_path}/{name}' if parent_path else name
        else:
            cache[group_id] = name
        return cache[group_id]

    for gid in normalized_ids:
        resolve_path(gid)
    return cache


def build_inventory_snapshot(host_ids: list[int], group_ids: list[int]) -> dict[str, Any]:
    hosts = _collect_hosts(host_ids, group_ids)
    snapshot_hosts: list[dict[str, Any]] = []
    snapshot_group_ids = [int(host.group_id) for host in hosts if host.group_id is not None]
    group_path_map = _build_group_path_map(snapshot_group_ids)

    for host in hosts:
        snapshot_hosts.append({
            'host_id': host.id,
            'host_name': str(host.instance_name).strip(),
            'host_ip': host.ip,
            'group_id': host.group_id,
            'group_name': host.group.name if getattr(host, 'group', None) else '',
            'group_path': group_path_map.get(int(host.group_id), '') if host.group_id is not None else '',
            # agent 在线状态（来自 DB 字段，由 runagentconsumer 维护）
            'agent_online': bool(getattr(host, 'agent_online', False)),
        })

    return {
        'selected_host_ids': host_ids,
        'selected_group_ids': group_ids,
        'hosts': snapshot_hosts,
    }


def execute_automation_job(job_id: int) -> None:
    # Ensure thread uses a valid DB connection lifecycle.
    close_old_connections()

    # Claim the job via an atomic state transition so duplicate deliveries cannot execute twice.
    start_time = timezone.now()
    claimed = AutomationExecutionJob.objects.filter(
        id=job_id,
        status=AutomationExecutionJob.Status.PENDING,
    ).update(
        status=AutomationExecutionJob.Status.RUNNING,
        start_time=start_time,
        result_summary={'message': 'Job is running'},
    )
    if claimed == 0:
        close_old_connections()
        return

    job = AutomationExecutionJob.objects.filter(id=job_id).first()
    if not job:
        close_old_connections()
        return

    template_content = (job.template_content_snapshot or '').strip()
    if not template_content:
        end_time = timezone.now()
        job.status = AutomationExecutionJob.Status.FAILED
        job.end_time = end_time
        job.duration_seconds = (end_time - start_time).total_seconds()
        job.result_summary = {
            'message': 'Template snapshot is empty. Cannot execute without immutable snapshot content.',
            'total': 0,
            'success': 0,
            'failed': 0,
        }
        job.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary'])
        close_old_connections()
        return

    # Detect template type: shell script or playbook
    is_shell_script = job.task and hasattr(job.task, 'shell_script_template_id') and job.task.shell_script_template_id

    if is_shell_script:
        # ===== SHELL SCRIPT EXECUTION PATH =====
        run_success, return_code = execute_shell_script_job(job)
        total_targets = len(job.inventory_snapshot.get('hosts', []) if isinstance(job.inventory_snapshot, dict) else [])
        shell_summary = job.result_summary if isinstance(job.result_summary, dict) else {}
        # Shell 路径已在 executor_shell_script 中写入 serial/fail-fast 统计。
        success_count = int(shell_summary.get('success', total_targets if run_success else 0) or 0)
        failed_count = int(shell_summary.get('failed', 0 if run_success else total_targets) or 0)
        skipped_count = int(shell_summary.get('skipped', 0) or 0)
        invalid_target_count = 0
    else:
        # ===== PLAYBOOK EXECUTION PATH =====
        # Global behavior change: Playbook execution is now delegated to dj-agent,
        # so HostCredential no longer participates in playbook task execution.
        snapshot_hosts = job.inventory_snapshot.get('hosts', []) if isinstance(job.inventory_snapshot, dict) else []
        hosts = [item for item in snapshot_hosts if isinstance(item, dict)]
        total_targets = len(hosts)
        if total_targets == 0:
            end_time = timezone.now()
            job.status = AutomationExecutionJob.Status.FAILED
            job.end_time = end_time
            job.duration_seconds = (end_time - start_time).total_seconds()
            job.result_summary = {
                'message': 'No target hosts found in inventory snapshot.',
                'total': total_targets,
                'success': 0,
                'failed': total_targets,
                'execution_mode': 'agent_grpc_sync',
            }
            job.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary'])
            close_old_connections()
            return

        latest_job = AutomationExecutionJob.objects.filter(id=job.id).values('status').first()
        if latest_job and latest_job.get('status') == AutomationExecutionJob.Status.CANCELLED:
            run_success = False
            return_code = -1
            success_count = 0
            failed_count = total_targets
        else:
            run_success, agent_summary, _ = execute_job_via_agent_grpc(
                automation_execution_job_id=int(getattr(job, 'pk', 0) or 0),
                automation_task_id=int(getattr(getattr(job, 'task', None), 'pk', 0) or 0),
                template_content=template_content,
                template_type='playbook',
                hosts=hosts,
                shell_parameters='',
                shell_env_vars={},
                extra_vars=job.extra_vars if isinstance(job.extra_vars, dict) else {},
                run_as_user=str(job.run_as_user_snapshot or ''),
                run_as_group=str(job.run_as_group_snapshot or ''),
                work_directory=str(job.work_directory_snapshot or '/tmp'),
                timeout_seconds=int(getattr(getattr(job, 'task', None), 'execution_timeout_seconds', 600) or 600),
            )
            success_count = int(agent_summary.get('success_count', 0) or 0)
            failed_count = int(agent_summary.get('failed_count', total_targets if not run_success else 0) or 0)
            return_code = 0 if run_success else 1
            merged_summary = {
                'message': str(agent_summary.get('message') or 'Execution finished'),
                'total': total_targets,
                'success': success_count,
                'failed': failed_count,
                'rc': return_code,
                'execution_mode': 'agent_grpc_sync',
                'created_count': int(agent_summary.get('created_count', 0) or 0),
                'failed_rows': agent_summary.get('failed_rows', []),
            }
            job.result_summary = merged_summary
            job.save(update_fields=['result_summary'])

    final_status = AutomationExecutionJob.Status.SUCCESS if failed_count == 0 else AutomationExecutionJob.Status.FAILED
    latest_status = AutomationExecutionJob.objects.filter(id=job.id).values_list('status', flat=True).first()
    if latest_status == AutomationExecutionJob.Status.CANCELLED:
        final_status = AutomationExecutionJob.Status.CANCELLED

    end_time = timezone.now()
    job.status = final_status
    job.end_time = end_time
    job.duration_seconds = (end_time - start_time).total_seconds()
    result_summary = {
        'message': 'Execution finished',
        'total': total_targets,
        'success': success_count,
        'failed': failed_count,
        'rc': return_code,
    }
    if is_shell_script:
        result_summary['skipped'] = int(locals().get('skipped_count', 0) or 0)
        result_summary['mode'] = 'serial_fail_fast'
    else:
        existing_summary = job.result_summary if isinstance(job.result_summary, dict) else {}
        if existing_summary:
            # Keep agent-side diagnostics (failed_rows/execution_mode/etc.) while
            # normalizing top-level counters for UI aggregation.
            existing_summary.update(result_summary)
            result_summary = existing_summary

    job.result_summary = result_summary
    job.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary'])

    close_old_connections()
