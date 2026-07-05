import io
import traceback

from django.utils import timezone

from assets.tasks import collect_all_hosts_info, cleanup_webssh_session_logs
from automation.tasks import cleanup_ansible_execution_logs
from audit.tasks import cleanup_login_audit_logs, cleanup_operation_audit_logs
from menu.models import SysMenu
from scheduler.models import ScheduledTask, ScheduledTaskLog
from sys_config.models import SysConfig

LOG_RETENTION_DAYS = 30
LOG_MAX_ROWS_PER_TASK = 2000
LOG_RETENTION_DAYS_KEY = 'sys.scheduler.log_retention_days'
LOG_MAX_ROWS_PER_TASK_KEY = 'sys.scheduler.log_max_rows_per_task'
SCHEDULER_ENABLED_KEY = 'sys.scheduler.enabled'
AUTOMATION_LOG_RETENTION_DAYS_KEY = 'sys.automation.logs.retention_days'
AUDIT_LOGIN_LOG_RETENTION_DAYS_KEY = 'sys.audit.login_logs.retention_days'
AUDIT_OPERATION_LOG_RETENTION_DAYS_KEY = 'sys.audit.operation_logs.retention_days'


def ensure_scheduler_log_configs():
    defaults = [
        {
            'key': LOG_RETENTION_DAYS_KEY,
            'value': '30',
            'default_value': '30',
            'value_type': 'int',
            'name': '调度日志保留天数',
            'description': '保留最近 N 天日志，0 表示不按天数清理',
            'is_readonly': False,
        },
        {
            'key': LOG_MAX_ROWS_PER_TASK_KEY,
            'value': '2000',
            'default_value': '2000',
            'value_type': 'int',
            'name': '每任务最大日志条数',
            'description': '每个任务最多保留 N 条日志，0 表示不按条数清理',
            'is_readonly': False,
        },
        {
            'key': SCHEDULER_ENABLED_KEY,
            'value': 'true',
            'default_value': 'true',
            'value_type': 'bool',
            'name': '调度总开关',
            'description': 'true 启用调度，false 停止调度分发',
            'is_readonly': False,
        },
        {
            'key': AUTOMATION_LOG_RETENTION_DAYS_KEY,
            'value': '30',
            'default_value': '30',
            'value_type': 'int',
            'name': '自动化执行日志保留天数',
            'description': '自动化作业与主机执行明细在数据库中的保留天数',
            'is_readonly': False,
        },
        {
            'key': AUDIT_LOGIN_LOG_RETENTION_DAYS_KEY,
            'value': '90',
            'default_value': '90',
            'value_type': 'int',
            'name': '登录日志保留天数',
            'description': '登录审计日志在数据库中的保留天数',
            'is_readonly': False,
        },
        {
            'key': AUDIT_OPERATION_LOG_RETENTION_DAYS_KEY,
            'value': '90',
            'default_value': '90',
            'value_type': 'int',
            'name': '操作日志保留天数',
            'description': '操作审计日志在数据库中的保留天数',
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


def _parse_bool(value):
    return str(value).strip().lower() in {'1', 'true', 'yes', 'y', 'on'}


def refresh_log_retention_config():
    global LOG_RETENTION_DAYS, LOG_MAX_ROWS_PER_TASK
    LOG_RETENTION_DAYS = max(0, _get_int_config_value(LOG_RETENTION_DAYS_KEY, 30))
    LOG_MAX_ROWS_PER_TASK = max(0, _get_int_config_value(LOG_MAX_ROWS_PER_TASK_KEY, 2000))


def is_scheduler_enabled():
    ensure_scheduler_log_configs()
    config = SysConfig.objects.filter(key=SCHEDULER_ENABLED_KEY).first()
    if not config:
        return True
    return _parse_bool(config.value)


def set_scheduler_enabled(enabled):
    ensure_scheduler_log_configs()
    value = 'true' if enabled else 'false'
    SysConfig.objects.update_or_create(
        key=SCHEDULER_ENABLED_KEY,
        defaults={
            'value': value,
            'default_value': 'true',
            'value_type': 'bool',
            'name': '调度总开关',
            'description': 'true 启用调度，false 停止调度分发',
            'is_readonly': False,
        },
    )


def cleanup_task_logs(task):
    if not task:
        return

    refresh_log_retention_config()

    if LOG_RETENTION_DAYS > 0:
        cutoff = timezone.now() - timezone.timedelta(days=LOG_RETENTION_DAYS)
        ScheduledTaskLog.objects.filter(task=task, run_time__lt=cutoff).delete()

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
        'cleanup_webssh_session_logs': '/audit/webssh',
        'cleanup_ansible_execution_logs': '/sys/automation/logs',
        'cleanup_login_audit_logs': '/audit/login',
        'cleanup_operation_audit_logs': '/audit/operation-log',
    }
    menu_path = mapping.get(code)
    if not menu_path:
        return None
    return SysMenu.objects.filter(path=menu_path).first()


def ensure_default_tasks():
    ensure_scheduler_log_configs()
    task_defs = [
        {
            'code': 'collect_all_hosts_info',
            'name': '主机信息采集',
            'description': '定时采集所有主机信息',
            'enabled': True,
            'interval_minutes': 15,
        },
        {
            'code': 'cleanup_webssh_session_logs',
            'name': 'WebSSH 会话日志清理',
            'description': '按保留天数清理过期 WebSSH 会话审计日志',
            'enabled': True,
            'interval_minutes': 24 * 60,
        },
        {
            'code': 'cleanup_ansible_execution_logs',
            'name': '自动化执行日志清理',
            'description': '按保留天数清理过期自动化作业日志与目标明细',
            'enabled': True,
            'interval_minutes': 24 * 60,
        },
        {
            'code': 'cleanup_login_audit_logs',
            'name': '登录日志清理',
            'description': '按保留天数清理登录审计日志',
            'enabled': True,
            'interval_minutes': 24 * 60,
        },
        {
            'code': 'cleanup_operation_audit_logs',
            'name': '操作日志清理',
            'description': '按保留天数清理操作审计日志',
            'enabled': True,
            'interval_minutes': 24 * 60,
        },
    ]

    first_task = None
    for item in task_defs:
        defaults = {
            'name': item['name'],
            'description': item['description'],
            'enabled': item['enabled'],
            'interval_minutes': item['interval_minutes'],
            'update_time': timezone.now().date(),
        }
        menu = get_task_menu(item['code'])
        if menu:
            defaults['menu'] = menu

        task, created = ScheduledTask.objects.get_or_create(
            code=item['code'],
            defaults=defaults,
        )

        if not created and task.menu is None and menu is not None:
            task.menu = menu  # type: ignore[assignment]
            task.save(update_fields=['menu'])

        if task.enabled and task.interval_minutes and task.next_run_time is None:
            calculate_next_run_time(task)

        if first_task is None:
            first_task = task

    # Remove deprecated combined audit cleanup task to avoid confusion in task center.
    ScheduledTask.objects.filter(code='cleanup_audit_logs').delete()

    return first_task


def calculate_next_run_time(task):
    if not task.enabled or not task.interval_minutes:
        if task.next_run_time is not None:
            task.next_run_time = None
            task.save(update_fields=['next_run_time'])
        return None

    now = timezone.now()
    if task.last_run_time:
        next_time = task.last_run_time + timezone.timedelta(minutes=task.interval_minutes)
        if next_time <= now:
            next_time = now + timezone.timedelta(minutes=task.interval_minutes)
    else:
        next_time = now + timezone.timedelta(minutes=task.interval_minutes)

    task.next_run_time = next_time
    task.save(update_fields=['next_run_time'])
    return next_time


def resolve_task_callable(task_code):
    if task_code == 'collect_all_hosts_info':
        return collect_all_hosts_info
    if task_code == 'cleanup_webssh_session_logs':
        return cleanup_webssh_session_logs
    if task_code == 'cleanup_ansible_execution_logs':
        return cleanup_ansible_execution_logs
    if task_code == 'cleanup_login_audit_logs':
        return cleanup_login_audit_logs
    if task_code == 'cleanup_operation_audit_logs':
        return cleanup_operation_audit_logs
    return None


def run_scheduled_task(task_code):
    task = ScheduledTask.objects.filter(code=task_code).first()
    if not task:
        raise ValueError(f"Task '{task_code}' not found")
    if task.is_running:
        return False

    func = resolve_task_callable(task_code)
    if func is None:
        raise ValueError(f'Unknown task code: {task_code}')

    start_time = timezone.now()
    status = '成功'
    message = '执行成功'
    output = ''
    old_stdout = io.StringIO()
    old_stderr = io.StringIO()

    task.is_running = True
    task.save(update_fields=['is_running'])

    try:
        from contextlib import redirect_stdout, redirect_stderr
        with redirect_stdout(old_stdout), redirect_stderr(old_stderr):
            func()
    except Exception as exc:
        status = '失败'
        message = str(exc) or '执行失败'
        output = old_stdout.getvalue()
        if old_stderr.getvalue():
            output += '\n[STDERR]\n' + old_stderr.getvalue()
        output += '\n[EXCEPTION]\n' + traceback.format_exc()
    finally:
        if not output:
            output = old_stdout.getvalue()
            if old_stderr.getvalue():
                output += '\n[STDERR]\n' + old_stderr.getvalue()

        now = timezone.now()
        task.refresh_from_db()
        task.last_run_time = now
        task.last_status = status
        task.last_message = message
        task.update_time = now.date()
        task.is_running = False
        task.save(update_fields=['last_run_time', 'last_status', 'last_message', 'update_time', 'is_running'])
        calculate_next_run_time(task)

        ScheduledTaskLog.objects.create(
            task=task,
            run_time=now,
            status=status,
            message=message,
            duration_seconds=(now - start_time).total_seconds(),
            output=output,
        )
        cleanup_task_logs(task)

    return status == '成功'
