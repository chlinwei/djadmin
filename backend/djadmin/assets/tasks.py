import logging
import warnings
from datetime import timedelta

from celery import shared_task
from django.utils import timezone
from cryptography.utils import CryptographyDeprecationWarning

from .models import WebSSHSessionLog, WebSSHTempCredential, Credential
from sys_config.models import SysConfig

logger = logging.getLogger(__name__)

warnings.filterwarnings(
    'ignore',
    message=r'.*TripleDES has been moved to cryptography\.hazmat\.decrepit\.ciphers\.algorithms\.TripleDES.*',
    category=CryptographyDeprecationWarning,
)

def cleanup_webssh_session_logs():
    """按系统参数清理过期 WebSSH 会话日志，供调度中心调用。"""
    retention_cfg, _ = SysConfig.objects.get_or_create(
        key='sys.audit.webssh.retention_days',
        defaults={
            'value': '30',
            'default_value': '30',
            'value_type': 'int',
            'name': 'WebSSH 审计保留天数',
            'description': 'WebSSH 会话完整记录在数据库中的保留天数',
            'is_readonly': False,
        },
    )

    try:
        retention_days = max(1, int(str(retention_cfg.value).strip()))
    except (ValueError, TypeError):
        retention_days = 30

    cutoff = timezone.now() - timedelta(days=retention_days)
    deleted_count, _ = WebSSHSessionLog.objects.filter(start_time__lt=cutoff).delete()
    print(f'[CLEANUP] WebSSH session logs cleaned: {deleted_count}, retention_days={retention_days}')
    return deleted_count


@shared_task(name='assets.cleanup_orphan_temp_credentials')
def cleanup_orphan_temp_credentials():
    """清理孤立的临时 WebSSH 凭证。

    删除条件（满足任一）：
    1. 已绑定 session，但该 session 不再活跃（已结束，或会话日志已被清理导致 session_pk 悬空）
    2. 创建超过 2 小时且从未绑定 session（session_pk is null）

    disconnect() 会主动删除正常关闭的临时凭证；此任务处理异常断开的兜底情况。
    """
    now = timezone.now()
    stale_cutoff = now - timedelta(hours=2)

    # 仍处于活跃状态的会话 id 集合（会话记录存在且未结束）。
    # 只有绑定到这些会话的临时凭证才需要保留，其余绑定关系一律视为孤立。
    closed_statuses = [WebSSHSessionLog.Status.CLOSED, WebSSHSessionLog.Status.FAILED]
    active_session_pks = set(
        WebSSHSessionLog.objects.exclude(
            status__in=closed_statuses
        ).values_list('id', flat=True)
    )

    # WebSSHTempCredential.credential 为 CASCADE，删除 Credential 会级联删除 TempCredential 与 HostCredential 关系；
    # 反向删除 TempCredential 并不会删掉 Credential，因此必须以 Credential 为删除入口。
    orphan_credential_ids = set()

    # 已绑定 session 的临时凭证：只要其 session 不在活跃集合中即视为孤立，
    # 覆盖两种情况——会话已结束(closed/failed)，或会话日志已被清理导致 session_pk 悬空。
    for cred_id, session_pk in WebSSHTempCredential.objects.filter(
        session_pk__isnull=False
    ).values_list('credential_id', 'session_pk'):
        if session_pk not in active_session_pks:
            orphan_credential_ids.add(cred_id)

    # 未绑定 session 且创建超过 2 小时的临时凭证（异常中断、从未成功建立会话的兜底清理）。
    orphan_credential_ids |= set(
        WebSSHTempCredential.objects.filter(
            session_pk__isnull=True,
            created_at__lt=stale_cutoff,
        ).values_list('credential_id', flat=True)
    )

    if not orphan_credential_ids:
        # 用 print 输出，便于 run_scheduled_task 通过 redirect_stdout 捕获进任务日志。
        print('[CLEANUP] Orphan temp credentials cleaned: 0 (无可清理的孤立临时凭证)')
        return 0

    cleaned_count = len(orphan_credential_ids)
    Credential.objects.filter(id__in=orphan_credential_ids).delete()
    # 同时输出到 stdout（写入定时任务日志）与 logger（应用日志）。
    print(f'[CLEANUP] Orphan temp credentials cleaned: {cleaned_count}')
    logger.info('[CLEANUP] Orphan temp credentials cleaned: %d', cleaned_count)
    return cleaned_count

