import json
import os
import stat
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from datetime import datetime
from typing import Any

from django.db import close_old_connections
from django.utils import timezone

from assets.models import Credential, Host, HostCredential, HostGroup

from .models import AnsibleExecutionJob, AnsibleExecutionTarget


@dataclass
class HostExecutionContext:
    host: Host
    credential: Credential
    alias: str


def _display_host_name(host: Host) -> str:
    system = getattr(host, 'system', None)
    hostname = getattr(system, 'hostname', None) if system else None
    return host.instance_name or hostname or f'Host-{host.id}'


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
        return []

    combined_ids = set()
    for condition in conditions:
        ids = queryset.filter(**condition).values_list('id', flat=True)
        combined_ids.update(ids)

    if not combined_ids:
        return []

    return list(queryset.filter(id__in=list(combined_ids)).order_by('id'))


def _get_default_credential(host: Host) -> Credential | None:
    relation = HostCredential.objects.filter(host=host, is_default=True).select_related('credential').first()
    if not relation or not relation.credential_id:
        return None
    return relation.credential


def build_inventory_snapshot(host_ids: list[int], group_ids: list[int]) -> dict[str, Any]:
    hosts = _collect_hosts(host_ids, group_ids)
    snapshot_hosts: list[dict[str, Any]] = []

    for host in hosts:
        snapshot_hosts.append({
            'host_id': host.id,
            'host_name': _display_host_name(host),
            'host_ip': host.ip,
            'group_id': host.group_id,
        })

    return {
        'selected_host_ids': host_ids,
        'selected_group_ids': group_ids,
        'hosts': snapshot_hosts,
    }


def _prepare_target_rows(job: AnsibleExecutionJob) -> list[HostExecutionContext]:
    snapshot_hosts = job.inventory_snapshot.get('hosts', []) if isinstance(job.inventory_snapshot, dict) else []
    host_ids = [item.get('host_id') for item in snapshot_hosts if isinstance(item, dict) and item.get('host_id')]
    host_map = {
        h.id: h for h in Host.objects.filter(id__in=host_ids).select_related('system').order_by('id')
    }

    contexts: list[HostExecutionContext] = []
    now_date = datetime.now().date()

    for item in snapshot_hosts:
        if not isinstance(item, dict):
            continue
        host_id = item.get('host_id')
        host = host_map.get(host_id)
        if not host:
            AnsibleExecutionTarget.objects.create(
                job=job,
                host_id=None,
                host_name=str(item.get('host_name') or ''),
                host_ip=str(item.get('host_ip') or ''),
                status=AnsibleExecutionTarget.Status.FAILED,
                stderr='Host not found during execution',
                create_time=now_date,
            )
            continue

        credential = _get_default_credential(host)
        if not credential:
            AnsibleExecutionTarget.objects.create(
                job=job,
                host=host,
                host_name=_display_host_name(host),
                host_ip=str(host.ip or ''),
                status=AnsibleExecutionTarget.Status.FAILED,
                stderr='Default credential not configured',
                create_time=now_date,
            )
            continue

        alias = f'host_{host.id}'
        AnsibleExecutionTarget.objects.create(
            job=job,
            host=host,
            host_name=_display_host_name(host),
            host_ip=str(host.ip or ''),
            status=AnsibleExecutionTarget.Status.PENDING,
            create_time=now_date,
        )
        contexts.append(HostExecutionContext(host=host, credential=credential, alias=alias))

    return contexts


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


def _resolve_ansible_playbook_command() -> list[str] | None:
    configured_path = os.getenv('ANSIBLE_PLAYBOOK_PATH', '').strip()
    if configured_path:
        return [configured_path]

    # Default to ansible-core module execution; does not depend on PATH command lookup.
    # If ansible-core is missing, subprocess stderr will provide guidance.
    return [sys.executable, '-m', 'ansible.cli.playbook']


def _run_single_host_playbook(job: AnsibleExecutionJob, target: AnsibleExecutionTarget, inventory_path: str, playbook_path: str, alias: str) -> None:
    now = timezone.now()
    target.status = AnsibleExecutionTarget.Status.RUNNING
    target.start_time = now
    target.save(update_fields=['status', 'start_time'])

    cmd_prefix = _resolve_ansible_playbook_command()
    cmd = [*cmd_prefix, '-i', inventory_path, playbook_path, '--limit', alias]
    if isinstance(job.extra_vars, dict) and job.extra_vars:
        cmd.extend(['--extra-vars', json.dumps(job.extra_vars, ensure_ascii=False)])

    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, encoding='utf-8')
        end_time = timezone.now()
        target.end_time = end_time
        if target.start_time:
            target.duration_seconds = (end_time - target.start_time).total_seconds()
        target.rc = proc.returncode
        target.stdout = proc.stdout or ''
        target.stderr = proc.stderr or ''
        if proc.returncode != 0 and os.name == 'nt' and 'WinError 1' in target.stderr:
            target.stderr = (
                f"{target.stderr}\n"
                'Current runtime is Windows. Run Ansible from Linux/WSL and set ANSIBLE_PLAYBOOK_PATH if needed.'
            )
        target.status = AnsibleExecutionTarget.Status.SUCCESS if proc.returncode == 0 else AnsibleExecutionTarget.Status.FAILED
        target.save(update_fields=['end_time', 'duration_seconds', 'rc', 'stdout', 'stderr', 'status'])
    except FileNotFoundError:
        end_time = timezone.now()
        target.end_time = end_time
        if target.start_time:
            target.duration_seconds = (end_time - target.start_time).total_seconds()
        target.rc = -1
        target.status = AnsibleExecutionTarget.Status.FAILED
        target.stderr = 'ansible-playbook command not found. Please install Ansible on server runtime.'
        target.save(update_fields=['end_time', 'duration_seconds', 'rc', 'status', 'stderr'])


def execute_ansible_job(job_id: int) -> None:
    # Ensure thread uses a valid DB connection lifecycle.
    close_old_connections()

    job = AnsibleExecutionJob.objects.select_related('template').filter(id=job_id).first()
    if not job:
        return

    if job.status == AnsibleExecutionJob.Status.CANCELLED:
        return

    start_time = timezone.now()
    job.status = AnsibleExecutionJob.Status.RUNNING
    job.start_time = start_time
    job.result_summary = {'message': 'Job is running'}
    job.save(update_fields=['status', 'start_time', 'result_summary'])

    contexts = _prepare_target_rows(job)
    if not contexts:
        end_time = timezone.now()
        job.status = AnsibleExecutionJob.Status.FAILED
        job.end_time = end_time
        job.duration_seconds = (end_time - start_time).total_seconds()
        job.result_summary = {
            'message': 'No executable targets. Check host selection and credentials.',
            'total': 0,
            'success': 0,
            'failed': 0,
        }
        job.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary'])
        close_old_connections()
        return

    with tempfile.TemporaryDirectory(prefix='djadmin_ansible_') as work_dir:
        playbook_path = os.path.join(work_dir, 'playbook.yml')
        with open(playbook_path, 'w', encoding='utf-8') as playbook_fp:
            playbook_fp.write(job.template.content)

        inventory_path, _ = _write_inventory_file(work_dir, contexts)

        for ctx in contexts:
            latest_job = AnsibleExecutionJob.objects.filter(id=job.id).values('status').first()
            if latest_job and latest_job.get('status') == AnsibleExecutionJob.Status.CANCELLED:
                break
            target = AnsibleExecutionTarget.objects.filter(job=job, host=ctx.host).order_by('-id').first()
            if not target:
                continue
            _run_single_host_playbook(job, target, inventory_path, playbook_path, ctx.alias)

    all_targets = list(AnsibleExecutionTarget.objects.filter(job=job))
    success_count = sum(1 for item in all_targets if item.status == AnsibleExecutionTarget.Status.SUCCESS)
    failed_count = sum(1 for item in all_targets if item.status in (
        AnsibleExecutionTarget.Status.FAILED,
        AnsibleExecutionTarget.Status.UNREACHABLE,
    ))

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
        'total': len(all_targets),
        'success': success_count,
        'failed': failed_count,
    }
    job.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary'])

    close_old_connections()
