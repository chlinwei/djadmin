import io
import re
import logging
from django.db import transaction
from .models import Host, HostCredential, HostSystem, HostHardware, HostDisk
from django.utils import timezone

logger = logging.getLogger(__name__)

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
        connect_kwargs['password'] = credential.password
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
        elif 'name or service not known' in error_msg.lower() or 'getaddrinfo failed' in error_msg.lower():
            raise ValueError(f'DNS 解析失败或网络不可达：{error_msg}')
        else:
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


def _collect_linux_info(host, credential):
    result = {}
    os_release = _run_ssh_command(host, credential, 'cat /etc/os-release 2>/dev/null || true')
    os_type, os_version = _parse_os_release(os_release)
    hostname = _run_ssh_command(host, credential, 'hostname')
    kernel_version = _run_ssh_command(host, credential, 'uname -r')
    cpu_cores = _run_ssh_command(host, credential, "grep -c '^processor' /proc/cpuinfo")
    cpu_model = _run_ssh_command(host, credential, "grep -m 1 'model name' /proc/cpuinfo | cut -d':' -f2 | sed 's/^ //'" )
    memory_mb = _run_ssh_command(host, credential, "free -m | awk '/^Mem:/ {print $2}'")
    disk_total = _run_ssh_command(host, credential, "df -BG --total --exclude-type=tmpfs --exclude-type=devtmpfs | tail -1 | awk '{print $2}'")
    disk_total_gb = _parse_size_to_gb(disk_total)

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
        'disk_total_gb': disk_total_gb,
        'architecture': _run_ssh_command(host, credential, 'uname -m'),
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

        # Strong guard: only keep rows with valid path-like device/mount and parseable sizes.
        # This prevents intermittent malformed `df` outputs from polluting HostDisk with blanks.
        if not device.startswith('/'):
            continue
        if not mount_point.startswith('/'):
            continue
        if parsed_size is None or parsed_used is None:
            continue
        if parsed_size <= 0:
            continue

        if parsed_size is None and parsed_used is None and not filesystem:
            continue

        disks.append({
            'device': device,
            'mount_point': mount_point,
            'size_gb': parsed_size,
            'used_gb': parsed_used,
            'filesystem': filesystem,
        })
    result['disks'] = disks
    return result


def collect_host_info(host):
    """采集主机信息，并把成功/失败状态持久化到 Host，便于在列表中突出无法连接的主机。"""
    try:
        result = _collect_host_info_impl(host)
    except Exception as e:
        host.collect_status = Host.CollectStatus.FAILED
        host.collect_message = str(e)
        host.collect_time = timezone.now()
        host.save(update_fields=['collect_status', 'collect_message', 'collect_time'])
        raise
    host.collect_status = Host.CollectStatus.SUCCESS
    host.collect_message = ''
    host.collect_time = timezone.now()
    host.save(update_fields=['collect_status', 'collect_message', 'collect_time'])
    return result


def _collect_host_info_impl(host):
    logger.info(f'[COLLECT] 开始采集主机: {host.name}({host.ip})')
    credential = HostCredential.objects.filter(host=host, is_default=True).select_related('credential').first()
    if not credential or not credential.credential:
        error = f'主机 {host.name}({host.ip}) 没有配置默认凭证'
        logger.error(f'[COLLECT] {error}')
        raise ValueError(error)
    credential = credential.credential
    
    if not credential.username:
        error = f'主机 {host.name}({host.ip}) 的凭证没有配置用户名'
        logger.error(f'[COLLECT] {error}')
        raise ValueError(error)
    
    if credential.auth_type == credential.AuthType.PASSWORD and not credential.password:
        error = f'主机 {host.name}({host.ip}) 的凭证没有配置密码'
        logger.error(f'[COLLECT] {error}')
        raise ValueError(error)
    
    if credential.auth_type == credential.AuthType.SSH_KEY and not credential.private_key:
        error = f'主机 {host.name}({host.ip}) 的凭证没有配置私钥'
        logger.error(f'[COLLECT] {error}')
        raise ValueError(error)

    try:
        logger.info(f'[COLLECT] 开始收集主机信息: {host.name}({host.ip}), 用户: {credential.username}')
        data = _collect_linux_info(host, credential)
        logger.info(f'[COLLECT] 成功收集主机信息: {host.name}({host.ip})')
    except Exception as e:
        error_msg = str(e)
        logger.error(f'[COLLECT] 主机 {host.name}({host.ip}) 采集失败: {error_msg}', exc_info=True)
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
                'collected_at': collected_at,
                'update_time': timezone.now(),
            }
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
            }
        )
        disk_objs = [HostDisk(host=host, **disk) for disk in data['disks']]
        if disk_objs:
            HostDisk.objects.filter(host=host).delete()
            HostDisk.objects.bulk_create(disk_objs)
        else:
            print(f'[WARN] Host {host.id} disk parsing returned empty result, keep existing HostDisk records.')

    return data


def collect_all_hosts_info():
    from django.utils import timezone
    
    print(f'[START] Starting host collection at {timezone.now()}')
    # Use filter(ip__isnull=False) only, as GenericIPAddressField doesn't work well with exclude(ip='')
    hosts = Host.objects.filter(ip__isnull=False)
    print(f'[INFO] Found {hosts.count()} hosts to collect')
    
    success_count = 0
    failed_count = 0
    
    for host in hosts:
        try:
            print(f'[COLLECTING] Host {host.id} ({host.ip}) - {host.name}')  # type: ignore
            collect_host_info(host)
            success_count += 1
            print(f'[SUCCESS] Host {host.id} collected successfully')  # type: ignore
        except Exception as e:
            failed_count += 1
            print(f'[ERROR] Host {host.id} failed: {e}')  # type: ignore
    
    print(f'[SUMMARY] Collection completed: {success_count} successful, {failed_count} failed')
    print(f'[END] Finished at {timezone.now()}')
    return True
