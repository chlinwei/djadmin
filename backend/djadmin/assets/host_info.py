"""主机信息落库与按需拉取的公共逻辑。

原先主机静态信息由 Celery 周期任务通过 SSH(paramiko) 采集，现改为用户打开主机列表/详情页时
按需拉取：视图调用 refresh_host_info，经 gRPC 让 agent 执行 get_host_info。

Agent 启动时不主动采集主机快照。
"""
import logging
import uuid
import hashlib
import json

from django.db import transaction
from django.utils import timezone
from django.utils.dateparse import parse_datetime

from .models import Host, HostSystem, HostHardware, HostDisk, HostRuntime
from .grpc_transfer.client import AgentChannelClient, AgentGrpcTransferError

logger = logging.getLogger(__name__)


def _to_float(value):
    """安全地把上报值转为 float，非法或缺失返回 None。"""
    if value is None or value == '':
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def _to_int(value):
    if value is None or value == '':
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def _normalize_disks(disks):
    """规整磁盘列表，确保同一语义数据生成稳定哈希（与顺序无关）。"""
    normalized = []
    rows = disks if isinstance(disks, list) else []
    for item in rows:
        if not isinstance(item, dict):
            continue
        device = str(item.get('device') or '').strip()
        if not device:
            continue
        filesystem = str(item.get('filesystem') or '').strip() or None
        # squashfs 多为系统镜像只读挂载（如 snap），不纳入业务磁盘分区采集/展示。
        if str(filesystem or '').lower() == 'squashfs':
            continue
        normalized.append({
            'device': device,
            'mount_point': str(item.get('mount_point') or '').strip() or None,
            'filesystem': filesystem,
            'size_gb': _to_float(item.get('size_gb')),
            'used_gb': _to_float(item.get('used_gb')),
            'usage_percent': _to_float(item.get('usage_percent')),
        })
    normalized.sort(key=lambda item: (item.get('device') or '', item.get('mount_point') or ''))
    return normalized


def _build_static_fingerprint(result_data):
    """为静态资产字段构造稳定指纹，用于去重落库。"""
    source = result_data if isinstance(result_data, dict) else {}
    static_payload = {
        'os_type': str(source.get('os_type') or source.get('os') or '').strip() or None,
        'os_version': str(source.get('os_version') or '').strip() or None,
        'os_id': str(source.get('os_id') or '').strip().lower() or None,
        'os_id_like': str(source.get('os_id_like') or '').strip().lower() or None,
        'os_version_id': str(source.get('os_version_id') or '').strip() or None,
        'kernel_version': str(source.get('kernel_version') or '').strip() or None,
        'hostname': str(source.get('hostname') or '').strip() or None,
        'agent_version': str(source.get('agent_version') or '').strip() or None,
        'cpu_count': _to_int(source.get('cpu_count')),
        'cpu_model': str(source.get('cpu_model') or '').strip() or None,
        'memory_total_gb': _to_float(source.get('memory_total_gb')),
        'arch': str(source.get('arch') or '').strip() or None,
        'os_timezone': str(source.get('os_timezone') or '').strip() or None,
        'os_utc_offset': str(source.get('os_utc_offset') or '').strip() or None,
        'disks': _normalize_disks(source.get('disks')),
    }
    encoded = json.dumps(static_payload, ensure_ascii=False, sort_keys=True, separators=(',', ':'))
    return hashlib.sha256(encoded.encode('utf-8')).hexdigest()


def _to_datetime(value):
    parsed = parse_datetime(str(value or '').strip())
    if parsed is not None and timezone.is_naive(parsed):
        parsed = timezone.make_aware(parsed, timezone.utc)
    return parsed


def persist_host_info(host, status, result_data, error='', now=None):
    """将一次 get_host_info 的结果落库到 Host/HostSystem/HostHardware/HostDisk。

    result_data 由 dj-agent 的 get_host_info 返回，包含：
    - 静态资产：os_type/os_version/kernel_version/cpu_model/cpu_count/arch/memory_total_gb/disks；
    - 动态指标：cpu_usage_percent/memory/disk_io/os_uptime_* 等写入 HostRuntime。

    返回 True 表示成功落库静态资产；失败/无数据返回 False。
    """
    now = now or timezone.now()
    is_success = str(status or '').strip() == 'success'
    static_fingerprint = ''
    if is_success and isinstance(result_data, dict) and result_data:
        static_fingerprint = _build_static_fingerprint(result_data)
        result_data = dict(result_data)

    # Host 主表只保存采集状态；失败时不覆盖上次成功的 collect_time。
    update_fields = ['collect_status', 'collect_message', 'update_time']
    host.collect_status = 'success' if is_success else 'failed'
    host.collect_message = str(error or '')
    if is_success:
        host.collect_time = now
        update_fields.append('collect_time')
    host.save(update_fields=update_fields)

    if not is_success or not isinstance(result_data, dict) or not result_data:
        return False

    previous_runtime = HostRuntime.objects.filter(host=host).only('static_fingerprint').first()
    previous_fingerprint = str(getattr(previous_runtime, 'static_fingerprint', '') or '').strip()
    HostRuntime.objects.update_or_create(
        host=host,
        defaults={
            'cpu_usage_percent': _to_float(result_data.get('cpu_usage_percent')),
            'cpu_times': result_data.get('cpu_times') if isinstance(result_data.get('cpu_times'), dict) else {},
            'memory_usage_percent': _to_float(result_data.get('memory_usage_percent')),
            'memory': result_data.get('memory') if isinstance(result_data.get('memory'), dict) else {},
            'disk_io': result_data.get('disk_io') if isinstance(result_data.get('disk_io'), (list, dict)) else [],
            'os_uptime_seconds': _to_int(result_data.get('os_uptime_seconds')),
            'os_boot_time': _to_datetime(result_data.get('os_boot_time')),
            'metrics_sample_window_ms': _to_int(result_data.get('metrics_sample_window_ms')),
            'static_fingerprint': static_fingerprint,
            'collected_at': now,
        },
    )
    if previous_fingerprint and previous_fingerprint == static_fingerprint:
        # 静态资产未变化：保留最新动态快照即可，跳过系统/硬件/磁盘表重复写入。
        return True

    # os_type 优先取发行版名称（os_release NAME，如 "Ubuntu"），回退到 GOOS（"linux"）。
    os_type = str(result_data.get('os_type') or result_data.get('os') or '').strip() or None

    HostSystem.objects.update_or_create(
        host=host,
        defaults={
            'os_type': os_type,
            'os_version': str(result_data.get('os_version') or '').strip() or None,
            'os_id': str(result_data.get('os_id') or '').strip().lower() or None,
            'os_id_like': str(result_data.get('os_id_like') or '').strip().lower() or None,
            'os_version_id': str(result_data.get('os_version_id') or '').strip() or None,
            'kernel_version': str(result_data.get('kernel_version') or '').strip() or None,
            'hostname': str(result_data.get('hostname') or '').strip() or None,
            'agent_version': str(result_data.get('agent_version') or '').strip() or None,
            'timezone_name': str(result_data.get('os_timezone') or '').strip() or None,
            'utc_offset': str(result_data.get('os_utc_offset') or '').strip() or None,
            'collector_source': 'agent',  # 固定来源：dj-agent 本地采集
            'collected_at': now,
        },
    )

    disks = _normalize_disks(result_data.get('disks'))
    result_data['disks'] = disks
    disk_total_gb = None
    if disks:
        total = sum((_to_float(d.get('size_gb')) or 0) for d in disks if isinstance(d, dict))
        disk_total_gb = round(total, 1) if total > 0 else None

    HostHardware.objects.update_or_create(
        host=host,
        defaults={
            'cpu_cores': _to_int(result_data.get('cpu_count')),
            'cpu_model': str(result_data.get('cpu_model') or '').strip() or None,
            'memory_gb': _to_float(result_data.get('memory_total_gb')),
            'disk_total_gb': disk_total_gb,
            'architecture': str(result_data.get('arch') or '').strip() or None,
            'collected_at': now,
        },
    )

    # 磁盘表按最新挂载全量重建，以清除已卸载/删除的分区，避免残留脏数据。
    with transaction.atomic():
        HostDisk.objects.filter(host=host).delete()
        rows = []
        for d in disks:
            if not isinstance(d, dict):
                continue
            device = str(d.get('device') or '').strip()
            if not device:
                continue
            rows.append(HostDisk(
                host=host,
                device=device,
                mount_point=str(d.get('mount_point') or '').strip() or None,
                size_gb=_to_float(d.get('size_gb')),
                used_gb=_to_float(d.get('used_gb')),
                filesystem=str(d.get('filesystem') or '').strip() or None,
            ))
        if rows:
            HostDisk.objects.bulk_create(rows)

    return True


def refresh_host_info(host, timeout_seconds=15):
    """按需经 gRPC 让指定主机的 agent 执行 get_host_info，并落库。

    返回统一结构：{host_id, updated, skipped, error}
    - skipped=True：因缺少 agent_id 或 agent 离线而未发起拉取（非错误）；
    - updated=True：成功拉取并落库静态资产；
    - error：失败原因（skipped/失败时填充）。
    """
    result = {'host_id': host.id, 'updated': False, 'skipped': False, 'error': ''}

    agent_id = str(host.agent_id or '').strip()
    if not agent_id:
        result['skipped'] = True
        result['error'] = '主机未配置 agent_id，无法定位 agent'
        return result

    # 只对在线 agent 发起拉取，离线直接跳过（避免同步等待超时拖慢页面加载）。
    if not host.agent_online:
        result['skipped'] = True
        result['error'] = 'agent 离线'
        return result

    try:
        client = AgentChannelClient(agent_id)
        resp = client.execute_automation(
            job_id=uuid.uuid4().hex,
            params={},
            timeout_seconds=timeout_seconds,
            task_type='inventory',
            action='get_host_info',
        )
    except AgentGrpcTransferError as exc:
        # 通道级失败：记录为一次采集失败，但不抛出，交由调用方汇总。
        persist_host_info(host, 'failed', {}, error=str(exc))
        result['error'] = str(exc)
        return result
    except Exception as exc:  # noqa: BLE001 - 兜底防止单台异常影响批量
        logger.error('refresh_host_info unexpected error for %s: %s', agent_id, exc, exc_info=True)
        result['error'] = str(exc)
        return result

    status = str(resp.get('status') or '').strip()
    result_data = resp.get('result_data') or {}
    error_message = str(resp.get('error_message') or '')
    updated = persist_host_info(host, status, result_data, error=error_message)
    result['updated'] = updated
    if not updated and error_message:
        result['error'] = error_message
    return result
