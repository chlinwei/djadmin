"""Shell script execution engine for automation jobs via dj-agent HTTP."""
from __future__ import annotations

from .agent_http_runner import execute_job_via_agent_http
from .models import AutomationExecutionJob


def execute_shell_script_job(job: AutomationExecutionJob) -> tuple[bool, int]:
    """Execute shell scripts on target hosts via agent HTTP and persist summary."""
    script_content = (job.template_content_snapshot or '').strip()
    if not script_content:
        job.result_summary = {'message': 'Script content snapshot is empty', 'execution_mode': 'agent_http_sync'}
        job.save(update_fields=['result_summary'])
        return False, 1

    snapshot_hosts = job.inventory_snapshot.get('hosts', []) if isinstance(job.inventory_snapshot, dict) else []
    hosts = [item for item in snapshot_hosts if isinstance(item, dict)]
    if not hosts:
        job.result_summary = {'message': 'No target hosts found', 'execution_mode': 'agent_http_sync'}
        job.save(update_fields=['result_summary'])
        return False, 1

    shell_parameters = str(job.shell_parameters or '').strip()
    shell_env_vars = job.shell_env_vars if isinstance(job.shell_env_vars, dict) else {}
    task = job.task
    task_id = int(getattr(task, 'pk', 0) or 0) if task is not None else 0
    timeout_seconds = int(task.execution_timeout_seconds or 600) if task is not None else 600
    job_pk = int(getattr(job, 'pk', 0) or 0)

    success, summary, _ = execute_job_via_agent_http(
        automation_execution_job_id=job_pk,
        automation_task_id=task_id,
        template_content=script_content,
        template_type='shell_script',
        hosts=hosts,
        shell_parameters=shell_parameters,
        shell_env_vars=shell_env_vars,
        extra_vars={},
        become_enabled=bool(job.become_enabled_snapshot),
        become_method=str(job.become_method_snapshot or 'sudo'),
        become_user=str(job.become_user_snapshot or 'root'),
        timeout_seconds=timeout_seconds,
    )

    job.result_summary = summary
    job.save(update_fields=['result_summary'])
    return success, 0 if success else 1
