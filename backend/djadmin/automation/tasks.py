from celery import shared_task
from datetime import timedelta

from django.db.models import Q
from django.utils import timezone

from sys_config.models import SysConfig

from .executor import execute_ansible_job
from .models import AnsibleExecutionJob


@shared_task(name='automation.execute_ansible_job')
def execute_ansible_job_task(job_id):
    execute_ansible_job(int(job_id))


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
        AnsibleExecutionJob.Status.SUCCESS,
        AnsibleExecutionJob.Status.FAILED,
        AnsibleExecutionJob.Status.CANCELLED,
    ]

    queryset = AnsibleExecutionJob.objects.filter(status__in=finished_statuses).filter(
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
