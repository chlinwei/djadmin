import sys
import io
import traceback
from django.db import connection
from apscheduler.schedulers.background import BackgroundScheduler
from apscheduler.triggers.interval import IntervalTrigger
from django.conf import settings
from django.utils import timezone
from assets.tasks import collect_all_hosts_info
from menu.models import SysMenu
from scheduler.models import ScheduledTask, ScheduledTaskLog
from sys_config.models import SysConfig

scheduler = BackgroundScheduler(timezone=settings.TIME_ZONE)

# Log retention policy. Loaded from sys_config.
LOG_RETENTION_DAYS = 30
LOG_MAX_ROWS_PER_TASK = 2000
LOG_RETENTION_DAYS_KEY = 'sys.scheduler.log_retention_days'
LOG_MAX_ROWS_PER_TASK_KEY = 'sys.scheduler.log_max_rows_per_task'


def ensure_scheduler_log_configs():
    """Ensure scheduler log retention configs exist in sys_config."""
    defaults = [
        {
            'key': LOG_RETENTION_DAYS_KEY,
            'value': '30',
            'value_type': 'int',
            'name': '调度日志保留天数',
            'description': '保留最近 N 天日志，0 表示不按天数清理',
            'is_readonly': False,
        },
        {
            'key': LOG_MAX_ROWS_PER_TASK_KEY,
            'value': '2000',
            'value_type': 'int',
            'name': '每任务最大日志条数',
            'description': '每个任务最多保留 N 条日志，0 表示不按条数清理',
            'is_readonly': False,
        },
    ]

    for item in defaults:
        key = item.pop('key')
        SysConfig.objects.get_or_create(key=key, defaults=item)


def _get_int_config_value(key, default_value):
    config = SysConfig.objects.filter(key=key).first()
    if not config:
        return default_value
    try:
        return int(str(config.value).strip())
    except (ValueError, TypeError):
        return default_value


def refresh_log_retention_config():
    """Refresh retention settings from sys_config."""
    global LOG_RETENTION_DAYS, LOG_MAX_ROWS_PER_TASK

    LOG_RETENTION_DAYS = max(0, _get_int_config_value(LOG_RETENTION_DAYS_KEY, 30))
    LOG_MAX_ROWS_PER_TASK = max(0, _get_int_config_value(LOG_MAX_ROWS_PER_TASK_KEY, 2000))


def cleanup_task_logs(task):
    """Cleanup old scheduler logs to prevent unbounded table growth."""
    if not task:
        return

    # Pick up latest config values so runtime changes in sys_config take effect.
    refresh_log_retention_config()

    # 1) Remove logs older than retention days.
    if LOG_RETENTION_DAYS > 0:
        cutoff = timezone.now() - timezone.timedelta(days=LOG_RETENTION_DAYS)
        ScheduledTaskLog.objects.filter(task=task, run_time__lt=cutoff).delete()

    # 2) Keep only latest N logs per task.
    if LOG_MAX_ROWS_PER_TASK > 0:
        keep_ids = list(
            ScheduledTaskLog.objects.filter(task=task)
            .order_by('-run_time', '-id')
            .values_list('id', flat=True)[:LOG_MAX_ROWS_PER_TASK]
        )
        if keep_ids:
            ScheduledTaskLog.objects.filter(task=task).exclude(id__in=keep_ids).delete()


def get_task_menu(code):
    mapping = {
        'collect_all_hosts_info': '/assets/hosts/index',
    }
    menu_path = mapping.get(code)
    if not menu_path:
        return None
    return SysMenu.objects.filter(path=menu_path).first()


def ensure_default_tasks():
    defaults = {
        'name': '主机信息采集',
        'description': '定时采集所有主机信息',
        'enabled': True,
        'interval_minutes': 15,
        'update_time': timezone.now().date(),
    }
    menu = get_task_menu('collect_all_hosts_info')
    if menu:
        defaults['menu'] = menu
    task, created = ScheduledTask.objects.get_or_create(
        code='collect_all_hosts_info',
        defaults=defaults,
    )
    if not created and task.menu is None and menu is not None:
        task.menu = menu
        task.save(update_fields=['menu'])


def calculate_next_run_time(task):
    """Calculate and update the next run time for a task"""
    if not task.enabled or not task.interval_minutes:
        return None
    
    # Calculate next run time based on last run time
    if task.last_run_time:
        next_time = task.last_run_time + timezone.timedelta(minutes=task.interval_minutes)
    else:
        # If never run, schedule for now + interval
        next_time = timezone.now() + timezone.timedelta(minutes=task.interval_minutes)
    
    task.next_run_time = next_time
    task.save(update_fields=['next_run_time'])
    return next_time


def re_register_task(task_code):
    """Re-register a task in APScheduler with updated interval"""
    try:
        task = ScheduledTask.objects.get(code=task_code)
        if not task.enabled or not task.interval_minutes:
            if task.next_run_time is not None:
                task.next_run_time = None
                task.save(update_fields=['next_run_time'])
            # Remove job if task is disabled or has no interval
            try:
                job_id = f'task_{task_code}'
                if scheduler.running and scheduler.get_job(job_id):
                    scheduler.remove_job(job_id)
                    print(f"Removed job for task '{task.name}'")
            except Exception:
                pass
            return False
        
        # Recalculate next run time
        calculate_next_run_time(task)
        
        # Remove old job if exists
        job_id = f'task_{task_code}'
        try:
            if scheduler.running and scheduler.get_job(job_id):
                scheduler.remove_job(job_id)
        except Exception:
            pass
        
        # Register new job with updated interval
        if scheduler.running:
            if task_code == 'collect_all_hosts_info':
                scheduler.add_job(
                    run_scheduled_task,
                    trigger=IntervalTrigger(minutes=task.interval_minutes),
                    id=job_id,
                    args=[task_code, collect_all_hosts_info],
                    replace_existing=True,
                )
                print(f"Re-registered job for task '{task.name}' with interval {task.interval_minutes} minutes")
                return True
    except ScheduledTask.DoesNotExist:
        print(f"Task '{task_code}' not found")
        return False
    except Exception as e:
        print(f"Error re-registering task: {e}")
        return False


def calculate_next_run_time(task):
    """Calculate and update the next run time for a task"""
    if not task.enabled or not task.interval_minutes:
        return None
    
    # Calculate next run time based on last run time
    if task.last_run_time:
        next_time = task.last_run_time + timezone.timedelta(minutes=task.interval_minutes)
    else:
        # If never run, schedule for now + interval
        next_time = timezone.now() + timezone.timedelta(minutes=task.interval_minutes)
    
    task.next_run_time = next_time
    task.save(update_fields=['next_run_time'])
    return next_time


def run_scheduled_task(task_code, func, *args, **kwargs):
    """Execute a scheduled task with proper database connection handling"""
    # Close old database connections in thread
    from django.db import close_old_connections
    close_old_connections()
    
    task = ScheduledTask.objects.filter(code=task_code).first()
    start_time = timezone.now()
    status = '成功'
    message = '执行成功'
    output = ''
    
    # Set task as running
    if task:
        task.is_running = True
        task.save(update_fields=['is_running'])
    
    # Capture stdout and stderr
    old_stdout = sys.stdout
    old_stderr = sys.stderr
    sys.stdout = io.StringIO()
    sys.stderr = io.StringIO()
    
    try:
        result = func(*args, **kwargs)
        # Get captured output
        output = sys.stdout.getvalue()
        if sys.stderr.getvalue():
            output += '\n[STDERR]\n' + sys.stderr.getvalue()
        return result
    except Exception as exc:
        status = '失败'
        message = str(exc) or '执行失败'
        output = sys.stdout.getvalue()
        if sys.stderr.getvalue():
            output += '\n[STDERR]\n' + sys.stderr.getvalue()
        output += '\n[EXCEPTION]\n' + traceback.format_exc()
        return False
    finally:
        # Restore stdout and stderr
        sys.stdout = old_stdout
        sys.stderr = old_stderr
        
        if task:
            try:
                now = timezone.now()
                
                # Refresh task from database to avoid stale data
                task.refresh_from_db()
                
                # Update task execution info
                task.last_run_time = now
                task.last_status = status
                task.last_message = message
                task.update_time = now.date()
                task.save(update_fields=['last_run_time', 'last_status', 'last_message', 'update_time'])
                
                # Calculate and update next run time
                calculate_next_run_time(task)
                
                # Ensure is_running is reset to False AFTER all other updates
                task.is_running = False
                task.save(update_fields=['is_running'])
                
                # Create task log
                ScheduledTaskLog.objects.create(
                    task=task,
                    run_time=now,
                    status=status,
                    message=message,
                    duration_seconds=(now - start_time).total_seconds(),
                    output=output,
                )

                cleanup_task_logs(task)
                
                # Explicitly commit transaction
                connection.commit()
                
            except Exception as e:
                # If error occurs in finally block, still try to reset is_running
                print(f"Error updating task status: {e}")
                try:
                    task.is_running = False
                    task.save(update_fields=['is_running'])
                except Exception:
                    pass


def start():
    # STATE_STOPPED = 0, STATE_RUNNING = 1
    if scheduler.running:
        print(f"Scheduler is already running")
        return scheduler

    ensure_scheduler_log_configs()
    refresh_log_retention_config()

    ensure_default_tasks()

    # Recalculate next_run_time on every startup to avoid stale values after downtime/restart.
    for task in ScheduledTask.objects.all():
        if task.enabled and task.interval_minutes:
            calculate_next_run_time(task)
        elif task.next_run_time is not None:
            task.next_run_time = None
            task.save(update_fields=['next_run_time'])

    # Remove existing jobs
    try:
        for job in scheduler.get_jobs():
            scheduler.remove_job(job.id)
    except Exception:
        pass

    # Add jobs for all enabled tasks with their configured intervals
    tasks = ScheduledTask.objects.filter(enabled=True)
    for task in tasks:
        interval_minutes = task.interval_minutes or 15  # Default to 15 if not set
        job_id = f'task_{task.code}'
        
        if task.code == 'collect_all_hosts_info':
            print(f"Adding job for task '{task.name}' with interval {interval_minutes} minutes")
            scheduler.add_job(
                run_scheduled_task,
                trigger=IntervalTrigger(minutes=interval_minutes),
                id=job_id,
                args=[task.code, collect_all_hosts_info],
                replace_existing=True,
            )
    
    print(f"Starting scheduler...")
    scheduler.start()
    print(f"Scheduler started successfully. Jobs: {len(scheduler.get_jobs())}")
    return scheduler


def shutdown():
    if scheduler.running:
        print(f"Shutting down scheduler...")
        scheduler.shutdown()
        print(f"Scheduler shut down successfully")
