import json
import os
import stat
import tempfile
import yaml
from dataclasses import dataclass
from datetime import datetime
from typing import Any

from django.db import close_old_connections
from django.utils import timezone

from assets.models import Credential, Host, HostCredential, HostGroup

from .models import AnsibleExecutionJob

try:
    import ansible_runner  # type: ignore[import-not-found]
except ImportError:
    ansible_runner = None


@dataclass
class HostExecutionContext:
    host: Host
    credential: Credential
    alias: str


def _build_inventory_alias(host: Host) -> str:
    display_name = str(host.instance_name).strip()
    host_ip = str(getattr(host, 'ip', '') or '').strip()
    alias_text = f'{display_name}({host_ip})'
    # Inventory host token cannot contain whitespace.
    return ' '.join(alias_text.split()).replace(' ', '_')


def _build_group_descendants(group_ids: list[int]) -> set[int]:
    if not group_ids:
        return set()

    id_set = set(group_ids)
    queue = list(group_ids)

    while queue:
        current = queue.pop(0)
        children = HostGroup.objects.filter(parent_id=current).values_list('id', flat=True)
        for child_id in children:
            if child_id not in id_set:
                id_set.add(child_id)
                queue.append(child_id)

    return id_set


def _collect_hosts(host_ids: list[int], group_ids: list[int]) -> list[Host]:
    queryset = Host.objects.filter(ip__isnull=False).select_related('group').order_by('id')

    conditions = []
    if host_ids:
        conditions.append({'id__in': host_ids})

    group_id_set = _build_group_descendants(group_ids)
    if group_id_set:
        conditions.append({'group_id__in': list(group_id_set)})

    if not conditions:
        # Empty scope means "all hosts with IP" for automation task execution.
        return list(queryset)

    combined_ids = set()
    for condition in conditions:
        ids = queryset.filter(**condition).values_list('id', flat=True)
        combined_ids.update(ids)

    if not combined_ids:
        return []

    return list(queryset.filter(id__in=list(combined_ids)).order_by('id'))


def _build_group_path_map(group_ids: list[int]) -> dict[int, str]:
    normalized_ids = [int(item) for item in group_ids if isinstance(item, int)]
    if not normalized_ids:
        return {}

    group_rows = list(HostGroup.objects.all().values('id', 'name', 'parent_id'))
    group_lookup = {int(item['id']): item for item in group_rows if item.get('id') is not None}
    cache: dict[int, str] = {}

    def resolve_path(group_id: int) -> str:
        if group_id in cache:
            return cache[group_id]
        row = group_lookup.get(group_id)
        if not row:
            cache[group_id] = ''
            return ''

        name = str(row.get('name') or '').strip()
        parent_id_raw = row.get('parent_id')
        parent_id = int(parent_id_raw) if isinstance(parent_id_raw, int) else None
        if parent_id and parent_id != group_id:
            parent_path = resolve_path(parent_id)
            cache[group_id] = f'{parent_path}/{name}' if parent_path else name
        else:
            cache[group_id] = name
        return cache[group_id]

    for gid in normalized_ids:
        resolve_path(gid)
    return cache


def _get_default_credential(host: Host) -> Credential | None:
    relation = HostCredential.objects.filter(host=host, is_default=True).select_related('credential').first()
    if not relation or not relation.credential_id:
        return None
    return relation.credential


def build_inventory_snapshot(host_ids: list[int], group_ids: list[int]) -> dict[str, Any]:
    hosts = _collect_hosts(host_ids, group_ids)
    snapshot_hosts: list[dict[str, Any]] = []
    snapshot_group_ids = [int(host.group_id) for host in hosts if host.group_id is not None]
    group_path_map = _build_group_path_map(snapshot_group_ids)

    for host in hosts:
        snapshot_hosts.append({
            'host_id': host.id,
            'host_name': str(host.instance_name).strip(),
            'host_ip': host.ip,
            'group_id': host.group_id,
            'group_name': host.group.name if getattr(host, 'group', None) else '',
            'group_path': group_path_map.get(int(host.group_id), '') if host.group_id is not None else '',
        })

    return {
        'selected_host_ids': host_ids,
        'selected_group_ids': group_ids,
        'hosts': snapshot_hosts,
    }


def _prepare_target_rows(job: AnsibleExecutionJob) -> tuple[list[HostExecutionContext], int]:
    snapshot_hosts = job.inventory_snapshot.get('hosts', []) if isinstance(job.inventory_snapshot, dict) else []
    host_ids = [item.get('host_id') for item in snapshot_hosts if isinstance(item, dict) and item.get('host_id')]
    host_map = {
        h.id: h for h in Host.objects.filter(id__in=host_ids).select_related('system').order_by('id')
    }

    contexts: list[HostExecutionContext] = []
    invalid_target_count = 0

    for item in snapshot_hosts:
        if not isinstance(item, dict):
            continue
        host_id = item.get('host_id')
        host = host_map.get(host_id)
        if not host:
            invalid_target_count += 1
            continue

        credential = _get_default_credential(host)
        if not credential:
            invalid_target_count += 1
            continue

        alias = _build_inventory_alias(host)
        contexts.append(HostExecutionContext(host=host, credential=credential, alias=alias))

    return contexts, invalid_target_count


def _write_inventory_file(work_dir: str, contexts: list[HostExecutionContext]) -> tuple[str, dict[str, str]]:
    inventory_path = os.path.join(work_dir, 'inventory.ini')
    private_key_files: dict[str, str] = {}

    lines = ['[all]']
    for ctx in contexts:
        host_ip = str(ctx.host.ip or '')
        port = ctx.credential.port or ctx.host.port or 22
        username = ctx.credential.username or 'root'

        parts = [
            ctx.alias,
            f'ansible_host={host_ip}',
            f'ansible_port={port}',
            f'ansible_user={username}',
            'ansible_connection=ssh',
            # Avoid noisy interpreter discovery warnings while keeping auto detection.
            'ansible_python_interpreter=auto_silent',
        ]

        if ctx.credential.auth_type == Credential.AuthType.PASSWORD:
            if ctx.credential.password:
                parts.append(f"ansible_password={ctx.credential.password}")
        else:
            if ctx.credential.private_key:
                key_file = os.path.join(work_dir, f'key_{ctx.host.id}.pem')
                with open(key_file, 'w', encoding='utf-8') as key_fp:
                    key_fp.write(ctx.credential.private_key)
                os.chmod(key_file, stat.S_IRUSR | stat.S_IWUSR)
                private_key_files[ctx.alias] = key_file
                parts.append(f'ansible_ssh_private_key_file={key_file}')

        lines.append(' '.join(parts))

    with open(inventory_path, 'w', encoding='utf-8') as fp:
        fp.write('\n'.join(lines) + '\n')

    return inventory_path, private_key_files


def _inject_become_config(template_content: str, job: AnsibleExecutionJob) -> str:
    """
    动态注入 become 配置到 playbook 中
    
    如果 job.become_enabled_snapshot 为 False，返回原始内容
    否则，为 playbook 中每个 play 添加 become 配置
    """
    if not job.become_enabled_snapshot:
        return template_content
    
    try:
        plays = yaml.safe_load(template_content)
        if not isinstance(plays, list):
            # 如果不是列表，尝试包装成列表
            if isinstance(plays, dict):
                plays = [plays]
            else:
                # 无法解析，返回原始内容
                return template_content
        
        # 为每个 play 添加 become 配置
        for play in plays:
            if isinstance(play, dict):
                play['become'] = job.become_enabled_snapshot
                play['become_method'] = job.become_method_snapshot
                play['become_user'] = job.become_user_snapshot
        
        # 转回 YAML 字符串
        result = yaml.dump(plays, default_flow_style=False, allow_unicode=True, sort_keys=False)
        return result
    except Exception:
        # YAML 解析失败，返回原始内容
        return template_content


def _parse_runner_stats(raw_stats: Any) -> dict[str, dict[str, int]]:
    stats_by_host: dict[str, dict[str, int]] = {}
    if not isinstance(raw_stats, dict):
        return stats_by_host

    required = ('ok', 'changed', 'unreachable', 'failed', 'skipped', 'rescued', 'ignored')
    for host_key, stat_value in raw_stats.items():
        if not isinstance(stat_value, dict):
            continue
        host_name = str(host_key or '').strip()
        if not host_name:
            continue

        normalized: dict[str, int] = {}
        for metric in required:
            value = stat_value.get(metric, 0)
            try:
                normalized[metric] = int(value)
            except (TypeError, ValueError):
                normalized[metric] = 0
        stats_by_host[host_name] = normalized

    return stats_by_host


def _run_playbook_for_inventory(
    job: AnsibleExecutionJob,
    contexts: list[HostExecutionContext],
    inventory_path: str,
    playbook_path: str,
) -> tuple[bool, int]:

    work_dir = os.path.dirname(playbook_path)

    host_label_lookup: dict[str, str] = {}
    for ctx in contexts:
        host_name = str(ctx.host.instance_name or '').strip()
        host_ip = str(ctx.host.ip or '').strip()
        host_label = f'{host_name}({host_ip})' if host_name and host_ip and host_name != host_ip else (host_name or host_ip or '-')
        for key in {ctx.alias, host_name, host_ip, host_label}:
            lowered = str(key or '').strip().lower()
            if lowered:
                host_label_lookup[lowered] = host_label

    def _normalize_event_type(event_name: str) -> str:
        normalized_event_name = str(event_name or '').strip().lower()
        if normalized_event_name == 'playbook_on_play_start':
            return 'play_start'
        if normalized_event_name == 'playbook_on_task_start':
            return 'task_start'
        if normalized_event_name == 'playbook_on_stats':
            return 'recap'
        if normalized_event_name.startswith('runner_on_') or normalized_event_name.startswith('runner_item_on_'):
            return 'task_result'
        if 'warning' in normalized_event_name:
            return 'warning'
        return 'log'

    def _resolve_host_label_from_runner_event(data: dict[str, Any]) -> str:
        event_data_raw = data.get('event_data')
        event_data = event_data_raw if isinstance(event_data_raw, dict) else {}
        delegated_raw = event_data.get('delegated_vars')
        delegated: dict[str, Any] = delegated_raw if isinstance(delegated_raw, dict) else {}

        host_candidates = [
            event_data.get('host'),
            event_data.get('host_name'),
            event_data.get('remote_addr'),
            delegated.get('ansible_host'),
            delegated.get('inventory_hostname'),
        ]

        for candidate in host_candidates:
            key = str(candidate or '').strip().lower()
            if not key:
                continue
            matched = host_label_lookup.get(key)
            if matched:
                return matched
        return '-'

    def _append_job_event_lines(
        stream_name: str,
        text_chunk: str,
        preferred_host_label: str = '-',
        event_type_hint: str = '',
        play_name_hint: str = '',
        task_name_hint: str = '',
    ) -> None:
        if not text_chunk:
            return

        host_label = preferred_host_label or '-'

        now_text = timezone.now().strftime('%Y-%m-%d %H:%M:%S')
        normalized_lines = []
        for line in text_chunk.splitlines(keepends=True):
            if line.endswith('\n'):
                normalized_lines.append(f'[{now_text}][{host_label}][{stream_name}] {line}')
            else:
                normalized_lines.append(f'[{now_text}][{host_label}][{stream_name}] {line}\n')

        if not normalized_lines:
            return

        job.job_output = (job.job_output or '') + ''.join(normalized_lines)
        job.save(update_fields=['job_output'])

    def _runner_event_handler(data: dict[str, Any]) -> bool:
        event_name = str(data.get('event') or '').strip()
        event_data_raw = data.get('event_data')
        event_data = event_data_raw if isinstance(event_data_raw, dict) else {}
        preferred_host_label = _resolve_host_label_from_runner_event(data)

        # Use ansible-runner event_data fields directly.
        play_name_hint = str(event_data.get('play') or event_data.get('play_name') or '').strip()
        task_name_hint = str(event_data.get('task') or event_data.get('task_name') or '').strip()

        raw_stdout = str(data.get('stdout') or '')
        if not raw_stdout:
            return True

        # Keep raw runner stdout for recap parsing while storing formatted lines for UI rendering.
        line = raw_stdout.rstrip('\n')
        if not line:
            return True
        _append_job_event_lines(
            'stdout',
            f'{line}\n',
            preferred_host_label=preferred_host_label,
            event_type_hint=_normalize_event_type(event_name),
            play_name_hint=play_name_hint,
            task_name_hint=task_name_hint,
        )
        return True

    try:
        if ansible_runner is None:
            raise ModuleNotFoundError('ansible-runner is not installed')

        run_result = ansible_runner.run(
            private_data_dir=work_dir,
            project_dir=work_dir,
            playbook=os.path.basename(playbook_path),
            inventory=inventory_path,
            extravars=job.extra_vars if isinstance(job.extra_vars, dict) and job.extra_vars else None,
            event_handler=_runner_event_handler,
        )

        stderr_text = ''
        artifact_dir = str(getattr(getattr(run_result, 'config', None), 'artifact_dir', '') or '')
        if artifact_dir:
            stderr_path = os.path.join(artifact_dir, 'stderr')
            if os.path.exists(stderr_path):
                with open(stderr_path, 'r', encoding='utf-8', errors='replace') as stderr_fp:
                    stderr_text = str(stderr_fp.read() or '')
                if stderr_text:
                    _append_job_event_lines('stderr', stderr_text)

        run_status = str(getattr(run_result, 'status', '') or '').strip().lower()
        run_success = run_status == 'successful'
        return_code = int(getattr(run_result, 'rc', 1) or 1)
        return run_success, return_code
    except Exception as exc:
        error_text = f'ansible-runner execution failed: {exc}'
        _append_job_event_lines('stderr', error_text)
        return False, -1


def execute_ansible_job(job_id: int) -> None:
    # Ensure thread uses a valid DB connection lifecycle.
    close_old_connections()

    # Claim the job via an atomic state transition so duplicate deliveries cannot execute twice.
    start_time = timezone.now()
    claimed = AnsibleExecutionJob.objects.filter(
        id=job_id,
        status=AnsibleExecutionJob.Status.PENDING,
    ).update(
        status=AnsibleExecutionJob.Status.RUNNING,
        start_time=start_time,
        result_summary={'message': 'Job is running'},
    )
    if claimed == 0:
        close_old_connections()
        return

    job = AnsibleExecutionJob.objects.filter(id=job_id).first()
    if not job:
        close_old_connections()
        return

    contexts, invalid_target_count = _prepare_target_rows(job)
    if not contexts:
        end_time = timezone.now()
        job.status = AnsibleExecutionJob.Status.FAILED
        job.end_time = end_time
        job.duration_seconds = (end_time - start_time).total_seconds()
        total_targets = invalid_target_count or len(job.inventory_snapshot.get('hosts', []) if isinstance(job.inventory_snapshot, dict) else [])
        job.result_summary = {
            'message': 'No executable targets. Check host selection and credentials.',
            'total': total_targets,
            'success': 0,
            'failed': total_targets,
        }
        job.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary'])
        close_old_connections()
        return

    with tempfile.TemporaryDirectory(prefix='djadmin_ansible_') as work_dir:
        playbook_path = os.path.join(work_dir, 'playbook.yml')
        template_content = (job.template_content_snapshot or '').strip()
        if not template_content:
            end_time = timezone.now()
            job.status = AnsibleExecutionJob.Status.FAILED
            job.end_time = end_time
            job.duration_seconds = (end_time - start_time).total_seconds()
            job.result_summary = {
                'message': 'Template snapshot is empty. Cannot execute without immutable snapshot content.',
                'total': 0,
                'success': 0,
                'failed': 0,
            }
            job.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary'])
            close_old_connections()
            return

        # 动态注入 become 配置到 playbook
        template_content = _inject_become_config(template_content, job)

        with open(playbook_path, 'w', encoding='utf-8') as playbook_fp:
            playbook_fp.write(template_content)

        inventory_path, _ = _write_inventory_file(work_dir, contexts)
        run_success = False
        return_code = -1

        latest_job = AnsibleExecutionJob.objects.filter(id=job.id).values('status').first()
        if not (latest_job and latest_job.get('status') == AnsibleExecutionJob.Status.CANCELLED):
            run_success, return_code = _run_playbook_for_inventory(job, contexts, inventory_path, playbook_path)

    total_targets = len(contexts) + invalid_target_count
    if run_success:
        success_count = len(contexts)
        failed_count = invalid_target_count
    else:
        success_count = 0
        failed_count = total_targets

    final_status = AnsibleExecutionJob.Status.SUCCESS if failed_count == 0 else AnsibleExecutionJob.Status.FAILED
    latest_status = AnsibleExecutionJob.objects.filter(id=job.id).values_list('status', flat=True).first()
    if latest_status == AnsibleExecutionJob.Status.CANCELLED:
        final_status = AnsibleExecutionJob.Status.CANCELLED

    end_time = timezone.now()
    job.status = final_status
    job.end_time = end_time
    job.duration_seconds = (end_time - start_time).total_seconds()
    job.result_summary = {
        'message': 'Execution finished',
        'total': total_targets,
        'success': success_count,
        'failed': failed_count,
        'rc': return_code,
    }
    job.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary'])

    close_old_connections()
