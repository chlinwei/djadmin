from celery import shared_task
from datetime import timedelta

from django.db.models import Q
from django.utils import timezone

from sys_config.models import SysConfig

from .models import AutomationExecutionJob


def cleanup_ansible_execution_logs():
    """按系统参数清理过期自动化执行日志（作业与目标明细）。"""
    retention_cfg, _ = SysConfig.objects.get_or_create(
        key='sys.automation.logs.retention_days',
        defaults={
            'value': '30',
            'default_value': '30',
            'value_type': 'int',
            'name': '自动化执行日志保留天数',
            'description': '自动化作业与主机执行明细在数据库中的保留天数',
            'is_readonly': False,
        },
    )

    try:
        retention_days = max(1, int(str(retention_cfg.value).strip()))
    except (ValueError, TypeError):
        retention_days = 30

    cutoff = timezone.now() - timedelta(days=retention_days)
    finished_statuses = [
        AutomationExecutionJob.Status.SUCCESS,
        AutomationExecutionJob.Status.FAILED,
        AutomationExecutionJob.Status.CANCELLED,
    ]

    queryset = AutomationExecutionJob.objects.filter(status__in=finished_statuses).filter(
        Q(end_time__lt=cutoff) |
        Q(end_time__isnull=True, create_time__lt=cutoff)
    )

    deleted_jobs = queryset.count()
    deleted_count, _ = queryset.delete()
    print(
        f'[CLEANUP] automation logs cleaned: jobs={deleted_jobs}, '
        f'total_deleted={deleted_count}, retention_days={retention_days}'
    )
    return deleted_jobs


def cleanup_workflow_run_logs():
    """按系统参数清理过期 Workflow 运行记录。"""
    from .models import AutomationWorkflowRun

    retention_cfg, _ = SysConfig.objects.get_or_create(
        key='sys.workflow.logs.retention_days',
        defaults={
            'value': '30',
            'default_value': '30',
            'value_type': 'int',
            'name': 'Workflow 运行记录保留天数',
            'description': 'Workflow 运行记录在数据库中的保留天数',
            'is_readonly': False,
        },
    )

    try:
        retention_days = max(1, int(str(retention_cfg.value).strip()))
    except (ValueError, TypeError):
        retention_days = 30

    cutoff = timezone.now() - timedelta(days=retention_days)
    finished_statuses = ['success', 'failed', 'cancelled']

    queryset = AutomationWorkflowRun.objects.filter(
        status__in=finished_statuses,
        start_time__lt=cutoff,
    )

    deleted_count = queryset.count()
    queryset.delete()
    print(
        f'[CLEANUP] workflow run logs cleaned: deleted={deleted_count}, '
        f'retention_days={retention_days}'
    )
    return deleted_count


@shared_task(name='automation.check_and_fail_stale_jobs')
def check_and_fail_stale_jobs():
    """定期检查超时的pending和running任务，自动标记为失败"""
    now = timezone.now()
    
    # 1. Pending超过5分钟 → 失败（MQ或worker可能不可用）
    pending_timeout = now - timedelta(minutes=5)
    stale_pending = AutomationExecutionJob.objects.filter(
        status=AutomationExecutionJob.Status.PENDING,
        create_time__lt=pending_timeout
    )
    pending_count = 0
    for job in stale_pending:
        job.status = AutomationExecutionJob.Status.FAILED
        job.result_summary = {
            'message': 'Pending timeout (5min): No worker picked up this job. '
                       'Check Celery worker and RabbitMQ status.',
            'fail_reason': 'worker_unavailable'
        }
        job.save(update_fields=['status', 'result_summary'])
        pending_count += 1
    
    # 2. Running超过task的timeout时间 → 失败（任务执行卡住了）
    running_jobs = AutomationExecutionJob.objects.filter(
        status=AutomationExecutionJob.Status.RUNNING
    )
    running_count = 0
    for job in running_jobs:
        # 从 task 获取 timeout；任务不存在时使用默认 10 分钟。
        timeout_seconds = 600
        task_ref = getattr(job, 'task', None)
        if task_ref is not None:
            try:
                timeout_seconds = int(getattr(task_ref, 'execution_timeout_seconds', 0) or 600)
            except Exception:
                pass
        
        running_timeout = now - timedelta(seconds=timeout_seconds)
        if job.create_time < running_timeout:
            job.status = AutomationExecutionJob.Status.FAILED
            job.result_summary = {
                'message': f'Running timeout ({timeout_seconds}s): Job execution exceeded maximum time. '
                           'Possible causes: SSH hang, network timeout, or infinite loop in playbook.',
                'fail_reason': 'execution_timeout',
                'timeout_seconds': timeout_seconds
            }
            job.save(update_fields=['status', 'result_summary'])
            running_count += 1
    
    if pending_count > 0 or running_count > 0:
        print(f'[STALE_JOBS] failed_pending={pending_count}, failed_running={running_count}')
    
    return {'pending_failed': pending_count, 'running_failed': running_count}
