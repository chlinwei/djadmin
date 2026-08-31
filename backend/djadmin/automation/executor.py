from typing import Any

from django.db import close_old_connections, connection
from django.utils import timezone

from assets.models import Host

from .models import AutomationExecutionJob
from .executor_playbook import execute_playbook_job
from .limit_utils import build_group_path_map


def _safe_close_old_connections() -> None:
    if not connection.in_atomic_block:
        close_old_connections()


def _collect_hosts(host_ids: list[int]) -> list[Host]:
    if not host_ids:
        return []
    return list(
        Host.objects.filter(id__in=host_ids, ip__isnull=False).select_related('group').order_by('id')
    )


def build_inventory_snapshot(host_ids: list[int]) -> dict[str, Any]:
    hosts = _collect_hosts(host_ids)
    snapshot_hosts: list[dict[str, Any]] = []
    snapshot_group_ids = [int(host.group_id) for host in hosts if getattr(host, 'group_id', None) is not None]  # type: ignore[attr-defined]
    group_path_map = build_group_path_map(snapshot_group_ids)

    for host in hosts:
        group_id_val = getattr(host, 'group_id', None)
        snapshot_hosts.append({
            'host_id': getattr(host, 'id', None),
            'host_name': str(host.instance_name).strip(),
            'host_ip': host.ip,
            'group_id': group_id_val,
            'group_name': host.group.name if getattr(host, 'group', None) else '',  # type: ignore[attr-defined]
            'group_path': group_path_map.get(int(group_id_val), '') if group_id_val is not None else '',
            # agent 在线状态由 gRPC Session 建立与断开同步到 DB。
            'agent_online': bool(getattr(host, 'agent_online', False)),
        })

    return {
        'selected_host_ids': host_ids,
        'hosts': snapshot_hosts,
    }


def execute_automation_job(job_id: int) -> None:
    # Ensure thread uses a valid DB connection lifecycle.
    _safe_close_old_connections()

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
        _safe_close_old_connections()
        return

    job = AutomationExecutionJob.objects.filter(id=job_id).first()
    if not job:
        _safe_close_old_connections()
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
        _safe_close_old_connections()
        return

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
            'execution_mode': 'backend_ansible',
        }
        job.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary'])
        _safe_close_old_connections()
        return

    job_pk = getattr(job, 'id', None) or getattr(job, 'pk', None)
    latest_job = AutomationExecutionJob.objects.filter(id=job_pk).values('status').first()
    if latest_job and latest_job.get('status') == AutomationExecutionJob.Status.CANCELLED:
        run_success = False
        return_code = -1
        success_count = 0
        failed_count = total_targets
    else:
        run_success, playbook_summary, _ = execute_playbook_job(job)
        success_count = int(playbook_summary.get('success', 0) or 0)
        failed_count = int(playbook_summary.get('failed', total_targets if not run_success else 0) or 0)
        return_code = 0 if run_success else 1
        job.result_summary = {
            'message': str(playbook_summary.get('message') or 'Execution finished'),
            'total': total_targets,
            'success': success_count,
            'failed': failed_count,
            'rc': return_code,
            'execution_mode': 'backend_ansible',
            'forks': int(playbook_summary.get('forks', 0) or 0),
            'failed_rows': playbook_summary.get('failed_rows', []),
            'failure_details': playbook_summary.get('failure_details', []),
        }
        job.save(update_fields=['result_summary'])

    final_status = AutomationExecutionJob.Status.SUCCESS if failed_count == 0 else AutomationExecutionJob.Status.FAILED
    latest_status = AutomationExecutionJob.objects.filter(id=job_pk).values_list('status', flat=True).first()
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
    existing_summary = job.result_summary if isinstance(job.result_summary, dict) else {}
    if existing_summary:
        existing_summary.update(result_summary)
        result_summary = existing_summary

    job.result_summary = result_summary
    job.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary'])

    _safe_close_old_connections()
