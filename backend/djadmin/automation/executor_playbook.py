from __future__ import annotations

import json
import os
import subprocess
import tempfile
from pathlib import Path

from assets.models import Host

from .controller_ssh import get_or_create_controller_ssh_key, sync_controller_public_key_to_agents
from .models import AutomationExecutionJob, AutomationExecutionTargetLog


def execute_playbook_job(job: AutomationExecutionJob) -> tuple[bool, dict, str]:
    """Run one multi-host playbook from backend after agents install the controller key."""
    snapshot = job.inventory_snapshot if isinstance(job.inventory_snapshot, dict) else {}
    requested = [item for item in snapshot.get('hosts', []) if isinstance(item, dict)]
    task = job.task
    concurrency = max(1, min(int(getattr(task, 'execution_concurrency', 10) or 10), 100))
    host_ids = [int(item['host_id']) for item in requested if str(item.get('host_id') or '').isdigit()]
    host_map = {
        int(getattr(host, 'pk', 0) or 0): host
        for host in Host.objects.filter(id__in=host_ids)
    }
    private_key, public_key = get_or_create_controller_ssh_key()

    eligible, failures = [], []
    for item in requested:
        host_id = int(item['host_id']) if str(item.get('host_id') or '').isdigit() else None
        host = host_map.get(host_id)
        agent_id = str(getattr(host, 'instance_name', '') or '').strip()
        if host is None or not agent_id or not host.ip:
            failures.append((host, host_id, 'host has no usable agent identity or IP'))
        else:
            eligible.append((host, agent_id))

    sync_failures = sync_controller_public_key_to_agents([agent_id for _, agent_id in eligible], public_key, concurrency)
    ready = [(host, agent_id) for host, agent_id in eligible if agent_id not in sync_failures]
    failures.extend((host, host.id, sync_failures[agent_id]) for host, agent_id in eligible if agent_id in sync_failures)

    for host, host_id, error in failures:
        AutomationExecutionTargetLog.objects.create(
            job=job, host=host, host_id_snapshot=host_id, host_name_snapshot=str(getattr(host, 'instance_name', '') or ''),
            host_ip_snapshot=str(getattr(host, 'ip', '') or ''), status='failed', error_message=error,
        )
    if not ready:
        return False, {'message': 'No target agent accepted the controller key', 'total': len(requested), 'success': 0, 'failed': len(failures), 'execution_mode': 'backend_ansible'}, ''

    with tempfile.TemporaryDirectory(prefix='djadmin-ansible-') as work_dir:
        work_path = Path(work_dir)
        key_path = work_path / 'controller_key'
        playbook_path = work_path / 'playbook.yml'
        inventory_path = work_path / 'inventory.ini'
        known_hosts_path = work_path / 'known_hosts'
        key_path.write_text(private_key, encoding='utf-8')
        os.chmod(key_path, 0o600)
        playbook_path.write_text((job.template_content_snapshot or '').rstrip() + '\n', encoding='utf-8')
        inventory_lines = ['[all]']
        for host, _ in ready:
            inventory_lines.append(f'host_{host.id} ansible_host={host.ip} ansible_user=root ansible_port=22')
        inventory_lines.extend([
            '', '[all:vars]', f'ansible_ssh_private_key_file={key_path}',
            f"ansible_ssh_common_args='-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile={known_hosts_path}'",
        ])
        inventory_path.write_text('\n'.join(inventory_lines) + '\n', encoding='utf-8')
        command = ['ansible-playbook', '-i', str(inventory_path), '--forks', str(concurrency), str(playbook_path)]
        extra_vars = job.extra_vars if isinstance(job.extra_vars, dict) else {}
        if extra_vars:
            command.extend(['--extra-vars', json.dumps(extra_vars, ensure_ascii=False)])
        run_as_user = str(job.run_as_user_snapshot or '').strip()
        if run_as_user and run_as_user != 'root':
            command.extend(['--become', '--become-user', run_as_user])
        try:
            completed = subprocess.run(command, cwd=work_dir, text=True, capture_output=True, timeout=int(getattr(task, 'execution_timeout_seconds', 600) or 600), check=False)
            stdout, stderr, return_code = completed.stdout, completed.stderr, completed.returncode
        except subprocess.TimeoutExpired as exc:
            timeout_stdout = exc.stdout.decode('utf-8', errors='replace') if isinstance(exc.stdout, bytes) else str(exc.stdout or '')
            timeout_stderr = exc.stderr.decode('utf-8', errors='replace') if isinstance(exc.stderr, bytes) else str(exc.stderr or '')
            stdout, stderr, return_code = timeout_stdout, timeout_stderr + '\nPlaybook execution timed out.', 124

    status = 'success' if return_code == 0 else 'failed'
    for host, _ in ready:
        AutomationExecutionTargetLog.objects.create(
            job=job, host=host, host_id_snapshot=host.id, host_name_snapshot=str(host.instance_name or ''),
            host_ip_snapshot=str(host.ip or ''), status=status, stdout=stdout, stderr=stderr, exit_code=return_code,
        )
    failed_count = len(failures) + (0 if return_code == 0 else len(ready))
    summary = {'message': 'Playbook executed by backend through platform SSH key', 'total': len(requested),
               'success': len(ready) if return_code == 0 else 0, 'failed': failed_count,
               'execution_mode': 'backend_ansible', 'forks': concurrency}
    return failed_count == 0, summary, stdout + (f'\n[stderr]\n{stderr}' if stderr else '')