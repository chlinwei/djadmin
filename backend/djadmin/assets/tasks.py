import io
import re
import logging
import warnings
from datetime import timedelta

from celery import shared_task
from django.db import transaction
from django.utils import timezone
from cryptography.utils import CryptographyDeprecationWarning

from .models import Host, HostCredential, HostSystem, HostHardware, HostDisk, WebSSHSessionLog, WebSSHTempCredential, Credential
from .credential_crypto import decrypt_secret
from sys_config.models import SysConfig

logger = logging.getLogger(__name__)

warnings.filterwarnings(
    'ignore',
    message='.*TripleDES has been moved to cryptography\.hazmat\.decrepit\.ciphers\.algorithms\.TripleDES.*',
    category=CryptographyDeprecationWarning,
)

try:
    import paramiko
    from paramiko.ssh_exception import AuthenticationException, SSHException
except ImportError:
    paramiko = None
    AuthenticationException = Exception  # type: ignore
    SSHException = Exception  # type: ignore


def _get_connection_port(host, credential):
    if host.port:
        return host.port
    if credential and credential.port:
        return credential.port
    return 22


def _run_ssh_command(host, credential, command, timeout=30):
    if paramiko is None:
        raise ImportError('paramiko is required for SSH host collection. Please install paramiko.')

    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())

    connect_kwargs = {
        'hostname': host.ip,
        'port': _get_connection_port(host, credential),
        'username': credential.username if credential else 'root',
        'timeout': timeout,
        'banner_timeout': timeout,
        'allow_agent': False,  # 禁止使用 SSH Agent
        'look_for_keys': False,  # 禁止查找本地密钥
    }

    if credential and credential.auth_type == credential.AuthType.SSH_KEY:
        key_data = credential.private_key or ''
        if not key_data:
            raise ValueError(f'Host {host.id} has SSH key auth but no private_key saved.')
        key_file = io.StringIO(key_data)
        pkey = paramiko.RSAKey.from_private_key(key_file)
        connect_kwargs['pkey'] = pkey
    elif credential and credential.auth_type == credential.AuthType.PASSWORD:
        connect_kwargs['password'] = decrypt_secret(credential.password)
    else:
        raise ValueError(f'Unsupported credential type for Host {host.id}.')

    try:
        client.connect(**connect_kwargs)
    except AuthenticationException as e:
        raise ValueError(f'SSH 认证失败：{str(e)}')
    except (SSHException, OSError) as e:
        error_msg = str(e)
        if 'refused' in error_msg.lower():
            raise ValueError(f'SSH 连接被拒绝：{error_msg}')
        if 'name or service not known' in error_msg.lower() or 'getaddrinfo failed' in error_msg.lower():
            raise ValueError(f'DNS 解析失败或网络不可达：{error_msg}')
        if 'authentication failed' in error_msg.lower() or 'permission denied' in error_msg.lower():
            raise ValueError(f'SSH 认证失败：{error_msg}')
        raise ValueError(f'SSH 连接失败：{error_msg}')

    stdin, stdout, stderr = client.exec_command(command, timeout=timeout)
    out = stdout.read().decode('utf-8', errors='ignore')
    err = stderr.read().decode('utf-8', errors='ignore')
    client.close()
    if err:
        raise RuntimeError(f'Command failed: {err.strip()}')
    return out.strip()


def _parse_os_release(text):
    os_type = None
    os_version = None
    for line in text.splitlines():
        if line.startswith('NAME='):
            os_type = line.split('=', 1)[1].strip('"')
        if line.startswith('VERSION='):
            os_version = line.split('=', 1)[1].strip('"')
    return os_type, os_version


def _parse_size_to_gb(size_text):
    """Parse size strings like 10G/1.5T/900M into GB float."""
    if not size_text:
        return None

    text = str(size_text).strip().upper()
    matched = re.match(r'^(\d+(?:\.\d+)?)([KMGT])$', text)
    if not matched:
        return None

    value = float(matched.group(1))
    unit = matched.group(2)
    factor = {
        'K': 1 / (1024 * 1024),
        'M': 1 / 1024,
        'G': 1,
        'T': 1024,
    }
    return round(value * factor[unit], 3)


def _should_ignore_disk_device(device):
    device_text = (device or '').strip()
    return bool(re.match(r'^/dev/sr\d+$', device_text))


def _collect_linux_info(host, credential):
    result = {}
    os_release = _run_ssh_command(host, credential, 'cat /etc/os-release 2>/dev/null || true')
    os_type, os_version = _parse_os_release(os_release)
    hostname = _run_ssh_command(host, credential, 'hostname')
    kernel_version = _run_ssh_command(host, credential, 'uname -r')
    cpu_cores = _run_ssh_command(host, credential, "grep -c '^processor' /proc/cpuinfo")
    cpu_model = _run_ssh_command(host, credential, "grep -m 1 'model name' /proc/cpuinfo | cut -d':' -f2 | sed 's/^ //'")
    memory_mb = _run_ssh_command(host, credential, "free -m | awk '/^Mem:/ {print $2}'")
    architecture = _run_ssh_command(host, credential, 'uname -m')

    result['system'] = {
        'os_type': os_type or 'Linux',
        'os_version': os_version or '',
        'kernel_version': kernel_version,
        'hostname': hostname,
        'agent_version': 'ssh-collector',
    }
    result['hardware'] = {
        'cpu_cores': int(cpu_cores) if cpu_cores.isdigit() else None,
        'cpu_model': cpu_model.strip() if cpu_model else None,
        'memory_gb': float(memory_mb) / 1024 if memory_mb.isdigit() else None,
        'disk_total_gb': None,
        'architecture': architecture,
    }

    # Use df --output to avoid brittle awk quoting and field-position issues.
    mount_output = _run_ssh_command(
        host,
        credential,
        'df -BG --output=source,target,size,used,fstype --exclude-type=tmpfs --exclude-type=devtmpfs | tail -n +2',
    )
    disks = []
    for line in mount_output.splitlines():
        parts = line.split()
        if len(parts) < 5:
            continue
        device, mount_point, size_gb, used_gb, filesystem = parts[:5]
        device = (device or '').strip()
        mount_point = (mount_point or '').strip()
        filesystem = (filesystem or '').strip()

        # Skip invalid/blank rows to avoid writing empty HostDisk records.
        if not device and not mount_point:
            continue

        parsed_size = _parse_size_to_gb(size_gb)
        parsed_used = _parse_size_to_gb(used_gb)

        # Keep only valid disk rows to avoid malformed df output polluting HostDisk.
        if not device.startswith('/'):
            continue
        if not mount_point.startswith('/'):
            continue
        if parsed_size is None or parsed_used is None:
            continue
        if parsed_size <= 0:
            continue
        if _should_ignore_disk_device(device):
            continue

        disks.append(
            {
                'device': device,
                'mount_point': mount_point,
                'size_gb': parsed_size,
                'used_gb': parsed_used,
                'filesystem': filesystem,
            }
        )

    result['hardware']['disk_total_gb'] = round(sum(disk['size_gb'] for disk in disks), 3) if disks else None
    result['disks'] = disks
    return result


def collect_host_info(host):
    """采集主机信息，并把成功/失败状态持久化到 Host，便于在列表中突出无法连接的主机。"""
    try:
        result = _collect_host_info_impl(host)
    except Exception as e:
        now = timezone.now()
        host.collect_status = Host.CollectStatus.FAILED
        host.collect_message = str(e)
        host.collect_time = now
        host.save(update_fields=['collect_status', 'collect_message', 'collect_time'])
        raise

    host.collect_status = Host.CollectStatus.SUCCESS
    host.collect_message = ''
    host.collect_time = timezone.now()
    host.save(update_fields=['collect_status', 'collect_message', 'collect_time'])
    return result


def _collect_host_info_impl(host):
    host_label = host.instance_name or f'Host-{host.id}'
    logger.info(f'[COLLECT] 开始采集主机: {host_label}({host.ip})')
    credential_relation = (
        HostCredential.objects.filter(host=host, is_default=True).select_related('credential').first()
    )
    if not credential_relation or not credential_relation.credential:
        error = f'主机 {host_label}({host.ip}) 没有配置默认凭证'
        logger.error(f'[COLLECT] {error}')
        raise ValueError(error)

    credential = credential_relation.credential

    if not credential.username:
        error = f'主机 {host_label}({host.ip}) 的凭证没有配置用户名'
        logger.error(f'[COLLECT] {error}')
        raise ValueError(error)

    if credential.auth_type == credential.AuthType.PASSWORD and not credential.password:
        error = f'主机 {host_label}({host.ip}) 的凭证没有配置密码'
        logger.error(f'[COLLECT] {error}')
        raise ValueError(error)

    if credential.auth_type == credential.AuthType.SSH_KEY and not credential.private_key:
        error = f'主机 {host_label}({host.ip}) 的凭证没有配置私钥'
        logger.error(f'[COLLECT] {error}')
        raise ValueError(error)

    try:
        logger.info(f'[COLLECT] 开始收集主机信息: {host_label}({host.ip}), 用户: {credential.username}')
        data = _collect_linux_info(host, credential)
        logger.info(f'[COLLECT] 成功收集主机信息: {host_label}({host.ip})')
    except Exception as e:
        error_msg = str(e)
        logger.error(f'[COLLECT] 主机 {host_label}({host.ip}) 采集失败: {error_msg}', exc_info=True)
        raise ValueError(error_msg)

    collected_at = timezone.now()

    with transaction.atomic():
        HostSystem.objects.update_or_create(
            host=host,
            defaults={
                'os_type': data['system']['os_type'],
                'os_version': data['system']['os_version'],
                'kernel_version': data['system']['kernel_version'],
                'hostname': data['system']['hostname'],
                'agent_version': data['system']['agent_version'],
                # 明确标记为 ssh 来源；agent 主动上报会覆盖为 agent。
                'collector_source': data['system'].get('collector_source') or 'ssh',
                'collected_at': collected_at,
                'update_time': timezone.now(),
            },
        )
        HostHardware.objects.update_or_create(
            host=host,
            defaults={
                'cpu_cores': data['hardware']['cpu_cores'],
                'cpu_model': data['hardware']['cpu_model'],
                'memory_gb': data['hardware']['memory_gb'],
                'disk_total_gb': data['hardware']['disk_total_gb'],
                'architecture': data['hardware']['architecture'],
                'collected_at': collected_at,
                'update_time': timezone.now(),
            },
        )
        disk_objs = [HostDisk(host=host, **disk) for disk in data['disks']]
        if disk_objs:
            HostDisk.objects.filter(host=host).delete()
            HostDisk.objects.bulk_create(disk_objs)
        else:
            print(f'[WARN] Host {host.id} disk parsing returned empty result, keep existing HostDisk records.')

    return data


def collect_all_hosts_info():
    def _get_invalid_credential_reason(host):
        relation = HostCredential.objects.filter(host=host, is_default=True).select_related('credential').first()
        if not relation or not relation.credential:
            return '未配置默认凭证'

        credential = relation.credential
        if not credential.username:
            return '默认凭证缺少用户名'
        if credential.auth_type == credential.AuthType.PASSWORD and not credential.password:
            return '默认凭证缺少密码'
        if credential.auth_type == credential.AuthType.SSH_KEY and not credential.private_key:
            return '默认凭证缺少私钥'
        return ''

    print(f'[START] Starting host collection at {timezone.now()}')
    hosts = Host.objects.filter(ip__isnull=False)
    print(f'[INFO] Found {hosts.count()} hosts to collect')

    success_count = 0
    failed_count = 0
    skipped_count = 0

    for host in hosts:
        invalid_reason = _get_invalid_credential_reason(host)
        if invalid_reason:
            skipped_count += 1
            host.collect_status = Host.CollectStatus.UNKNOWN
            host.collect_message = f'定时采集已跳过：{invalid_reason}'
            host.collect_time = timezone.now()
            host.save(update_fields=['collect_status', 'collect_message', 'collect_time'])
            print(f'[SKIPPED] Host {host.id} skipped: {invalid_reason}')  # type: ignore
            continue

        try:
            host_label = host.instance_name or f'Host-{host.id}'
            print(f'[COLLECTING] Host {host.id} ({host.ip}) - {host_label}')  # type: ignore
            collect_host_info(host)
            success_count += 1
            print(f'[SUCCESS] Host {host.id} collected successfully')  # type: ignore
        except Exception as e:
            failed_count += 1
            print(f'[ERROR] Host {host.id} failed: {e}')  # type: ignore

    print(
        f'[SUMMARY] Collection completed: {success_count} successful, {failed_count} failed, {skipped_count} skipped'
    )
    print(f'[END] Finished at {timezone.now()}')
    return True


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


@shared_task(name='assets.sync_monitor_target_status_from_automation_jobs')
def sync_monitor_target_status_from_automation_jobs():
    """定期把 MonitorTarget.last_install_job_id 指向的 AutomationExecutionJob 最终状态
    同步回 install_status/install_message。

    背景：安装/卸载已改为下发到独立的“自动化任务”执行链路（AutomationExecutionJob，
    在本地后台线程里异步跑 ansible playbook），dispatch_exporter_install_job/uninstall_job
    下发后立即把 install_status 置为 pending 并返回，不等待、也不会被同步回调通知结果。
    若不主动轮询同步，install_status 会永远停在 pending：前端“重试”按钮只在
    install_status == 'failed' 时可点，用户将永远无法自行恢复。
    AutomationExecutionJob 自身的 pending/running 超时兜底由 automation.check_and_fail_stale_jobs
    负责（会把卡住的任务标记为 failed），这里只需要在下一轮轮询里把已经有终态
    （success/failed/cancelled）的任务结果映射回 MonitorTarget。"""
    from monitor.models import MonitorTarget
    from automation.models import AutomationExecutionJob

    pending_targets = MonitorTarget.objects.filter(
        install_status=MonitorTarget.InstallStatus.PENDING,
        last_install_job_id__isnull=False,
    )
    synced_count = 0
    for target in pending_targets:
        job = AutomationExecutionJob.objects.filter(id=target.last_install_job_id).first()
        if job is None:
            continue
        if job.status == AutomationExecutionJob.Status.SUCCESS:
            target.install_status = MonitorTarget.InstallStatus.SUCCESS
            target.install_message = f'automation job #{job.id} 执行成功'  # type: ignore[attr-defined]
            target.retry_count = 0
            target.save(update_fields=['install_status', 'install_message', 'retry_count', 'update_time'])
            synced_count += 1
        elif job.status in (AutomationExecutionJob.Status.FAILED, AutomationExecutionJob.Status.CANCELLED):
            summary = job.result_summary if isinstance(job.result_summary, dict) else {}
            target.install_status = MonitorTarget.InstallStatus.FAILED
            target.install_message = f'automation job #{job.id} 执行失败：{summary.get("message", "")}'  # type: ignore[attr-defined]
            target.save(update_fields=['install_status', 'install_message', 'update_time'])
            synced_count += 1

    if synced_count:
        print(f'[SYNC_MONITOR_TARGET] synced={synced_count}')
    return synced_count
