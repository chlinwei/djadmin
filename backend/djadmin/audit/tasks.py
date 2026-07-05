from datetime import timedelta

from django.utils import timezone

from sys_config.models import SysConfig

from .models import LoginAuditLog, OperationAuditLog


LOGIN_RETENTION_KEY = 'sys.audit.login_logs.retention_days'
OPERATION_RETENTION_KEY = 'sys.audit.operation_logs.retention_days'


def _get_retention_days(key, default_name, default_description, default_value=90):
    cfg, _ = SysConfig.objects.get_or_create(
        key=key,
        defaults={
            'value': str(default_value),
            'default_value': str(default_value),
            'value_type': 'int',
            'name': default_name,
            'description': default_description,
            'is_readonly': False,
        },
    )
    try:
        return max(1, int(str(cfg.value).strip()))
    except (TypeError, ValueError):
        return default_value


def cleanup_login_audit_logs():
    """按系统参数清理登录日志。"""
    login_days = _get_retention_days(
        LOGIN_RETENTION_KEY,
        '登录日志保留天数',
        '登录审计日志在数据库中的保留天数',
    )

    now = timezone.now()
    login_cutoff = now - timedelta(days=login_days)

    deleted_login, _ = LoginAuditLog.objects.filter(login_time__lt=login_cutoff).delete()
    print(
        '[CLEANUP] login audit logs cleaned: '
        f'deleted={deleted_login}, retention_days={login_days}'
    )
    return {
        'deleted_login': deleted_login,
        'login_retention_days': login_days,
    }


def cleanup_operation_audit_logs():
    """按系统参数清理操作日志。"""
    operation_days = _get_retention_days(
        OPERATION_RETENTION_KEY,
        '操作日志保留天数',
        '操作审计日志在数据库中的保留天数',
    )

    now = timezone.now()
    operation_cutoff = now - timedelta(days=operation_days)

    deleted_operation, _ = OperationAuditLog.objects.filter(created_at__lt=operation_cutoff).delete()
    print(
        '[CLEANUP] operation audit logs cleaned: '
        f'deleted={deleted_operation}, retention_days={operation_days}'
    )
    return {
        'deleted_operation': deleted_operation,
        'operation_retention_days': operation_days,
    }


