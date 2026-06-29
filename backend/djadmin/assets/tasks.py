import io
import re
from django.db import transaction
from .models import Host, HostCredential, HostSystem, HostHardware, HostDisk
from django.utils import timezone

try:
    import paramiko
except ImportError:
    paramiko = None


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

    client.connect(**connect_kwargs)
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
    disk_total_gb = None
    if disk_total.endswith('G'):
        disk_total_gb = float(disk_total[:-1])

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
    mount_output = _run_ssh_command(host, credential, 'df -BG --total --exclude-type=tmpfs --exclude-type=devtmpfs | awk "NR>1 {print $1\\",\\"$6\\",\\"$2\\",\\"$3\\",\\"$5}"')
    disks = []
    for line in mount_output.splitlines():
        parts = line.split(',')
        if len(parts) < 5:
            continue
        device, mount_point, size_gb, used_gb, filesystem = parts
        disks.append({
            'device': device,
            'mount_point': mount_point,
            'size_gb': float(size_gb[:-1]) if size_gb.endswith('G') else None,
            'used_gb': float(used_gb[:-1]) if used_gb.endswith('G') else None,
            'filesystem': filesystem,
        })
    result['disks'] = disks
    return result


def collect_host_info(host):
    credential = HostCredential.objects.filter(host=host, is_default=True).select_related('credential').first()
    if not credential or not credential.credential:
        raise ValueError(f'Host {host.id} has no default credential configured.')
    credential = credential.credential

    data = _collect_linux_info(host, credential)

    with transaction.atomic():
        HostSystem.objects.update_or_create(
            host=host,
            defaults={
                'os_type': data['system']['os_type'],
                'os_version': data['system']['os_version'],
                'kernel_version': data['system']['kernel_version'],
                'hostname': data['system']['hostname'],
                'agent_version': data['system']['agent_version'],
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
                'update_time': timezone.now(),
            }
        )
        HostDisk.objects.filter(host=host).delete()
        disk_objs = [HostDisk(host=host, **disk) for disk in data['disks']]
        HostDisk.objects.bulk_create(disk_objs)

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
            print(f'[COLLECTING] Host {host.id} ({host.ip}) - {host.name}')
            collect_host_info(host)
            success_count += 1
            print(f'[SUCCESS] Host {host.id} collected successfully')
        except Exception as e:
            failed_count += 1
            print(f'[ERROR] Host {host.id} failed: {e}')
    
    print(f'[SUMMARY] Collection completed: {success_count} successful, {failed_count} failed')
    print(f'[END] Finished at {timezone.now()}')
    return True
