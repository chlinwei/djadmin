from celery import shared_task
from django.utils import timezone

from scheduler.celery_health import has_active_celery_worker
from scheduler.models import ScheduledTask
from scheduler_manager import (
    calculate_next_run_time,
    ensure_default_tasks,
    is_scheduler_enabled,
    run_scheduled_task,
)


@shared_task(name='scheduler.execute_scheduled_task')
def execute_scheduled_task(task_code):
    return run_scheduled_task(task_code)


@shared_task(name='scheduler.dispatch_due_tasks')
def dispatch_due_tasks():
    ensure_default_tasks()
    if not is_scheduler_enabled():
        return {'status': 'disabled'}

    now = timezone.now()
    queued = 0
    tasks = ScheduledTask.objects.filter(enabled=True, interval_minutes__gt=0).order_by('id')
    worker_online = has_active_celery_worker()
    for task in tasks:
        if task.is_running:
            continue

        if task.next_run_time is None:
            calculate_next_run_time(task)
            task.refresh_from_db()

        if task.next_run_time and task.next_run_time <= now:
            if not worker_online:
                task.last_status = '失败'
                task.last_message = 'Celery worker 未启动或不可用，任务未投递'
                task.update_time = now.date()
                task.save(update_fields=['last_status', 'last_message', 'update_time'])
                continue
            # Advance next_run_time on dispatch to avoid repeated queueing while worker picks up.
            task.next_run_time = now + timezone.timedelta(minutes=task.interval_minutes or 15)
            task.save(update_fields=['next_run_time'])
            execute_scheduled_task.delay(task.code)
            queued += 1

    if not worker_online:
        return {'status': 'worker_unavailable', 'queued': 0}
    return {'status': 'ok', 'queued': queued}
