from __future__ import annotations

import json
import os
import re
import subprocess
import tempfile
from pathlib import Path
from typing import Any

from assets.models import Host

from .controller_ssh import get_or_create_controller_ssh_key, sync_controller_public_key_to_agents
from .models import AutomationExecutionTargetLog
from .ansible_runtime import get_ansible_playbook_command


def extract_ansible_error_summary(stdout: str, stderr: str) -> str:
    """从 Ansible 输出中解析出最精炼的错误摘要，便于在列表和通知中直观展示真实原因。"""
    combined = f'{stdout}\n{stderr}'.strip()
    if not combined:
        return 'Playbook execution failed (no output)'

    # 1. 尝试匹配 fatal: [host]: FAILED! => { "msg": "...", "stderr": "..." }
    fatal_match = re.search(r'fatal:\s*\[[^\]]+\]:\s*FAILED!\s*=>\s*(\{.*?\})', combined, re.DOTALL)
    if fatal_match:
        payload_str = fatal_match.group(1)
        try:
            payload = json.loads(payload_str)
            if isinstance(payload, dict):
                # 优先提取 stderr_lines 的核心行
                stderr_lines = payload.get('stderr_lines')
                if isinstance(stderr_lines, list) and stderr_lines:
                    meaningful = [
                        line.strip() for line in stderr_lines
                        if line.strip() and not line.strip().startswith('warning:')
                    ]
                    if meaningful:
                        return '\n'.join(meaningful[:3])

                # 其次提取 msg
                msg = payload.get('msg')
                if msg and str(msg).strip() and str(msg).strip() != 'non-zero return code':
                    return str(msg).strip()

                # 再次提取 stderr
                err = payload.get('stderr')
                if err and str(err).strip():
                    err_lines = [
                        line.strip() for line in str(err).splitlines()
                        if line.strip() and not line.strip().startswith('warning:')
                    ]
                    if err_lines:
                        return '\n'.join(err_lines[:3])
        except Exception:
            pass

    # 2. 匹配没有转为 JSON 的常见报错行
    for line in combined.splitlines():
        line_clean = line.strip()
        if not line_clean or line_clean.startswith('warning:'):
            continue
        if any(marker in line_clean.lower() for marker in ['failed dependencies:', 'error:', 'failed:', 'fatal:']):
            return line_clean

    return 'Playbook executed by backend with failure'


def run_ansible_playbook_on_hosts(
    *,
    hosts: list[Host],
    template_content: str,
    extra_vars: dict[str, Any] | None = None,
    run_as_user: str = 'root',
    concurrency: int = 10,
    timeout_seconds: int = 600,
) -> tuple[bool, dict[str, Any], str, list[tuple[Host | None, int | None, str]], list[tuple[Host, str]]]:
    """底层通用 Ansible 进程调用器：同步 Controller 公钥，以临时 inventory 与临时工作目录执行 Playbook。

    返回: (is_success, summary_dict, combined_output, failures_list, ready_list)
    """
    effective_concurrency = max(1, min(int(concurrency or 10), 100))
    private_key, public_key = get_or_create_controller_ssh_key()

    eligible: list[tuple[Host, str]] = []
    failures: list[tuple[Host | None, int | None, str]] = []
    for host in hosts:
        agent_id = str(getattr(host, 'agent_id', '') or '').strip()
        host_id = getattr(host, 'pk', None) or getattr(host, 'id', None)
        if host is None or not agent_id or not getattr(host, 'ip', None):
            failures.append((host, host_id, 'host has no usable agent identity or IP'))
        else:
            eligible.append((host, agent_id))

    sync_failures = sync_controller_public_key_to_agents(
        [agent_id for _, agent_id in eligible], public_key, effective_concurrency,
    )
    ready = [(host, agent_id) for host, agent_id in eligible if agent_id not in sync_failures]
    failures.extend(
        (host, getattr(host, 'pk', None) or getattr(host, 'id', None), sync_failures[agent_id])
        for host, agent_id in eligible if agent_id in sync_failures
    )

    total_requested = len(hosts)
    if not ready:
        failure_details = [
            f'{getattr(host, "instance_name", None) or host_id}: {error}'
            for host, host_id, error in failures
        ]
        summary = {
            'message': 'No target agent accepted the controller key',
            'failure_details': failure_details,
            'total': total_requested,
            'success': 0,
            'failed': len(failures),
            'execution_mode': 'backend_ansible',
        }
        return False, summary, '', failures, []

    with tempfile.TemporaryDirectory(prefix='djadmin-ansible-') as work_dir:
        work_path = Path(work_dir)
        key_path = work_path / 'controller_key'
        playbook_path = work_path / 'playbook.yml'
        inventory_path = work_path / 'inventory.ini'
        known_hosts_path = work_path / 'known_hosts'
        key_path.write_text(private_key, encoding='utf-8')
        os.chmod(key_path, 0o600)
        playbook_path.write_text((template_content or '').rstrip() + '\n', encoding='utf-8')
        inventory_lines = ['[all]']
        used_inventory_names: set[str] = set()
        for host, _ in ready:
            instance_name = str(getattr(host, 'instance_name', '') or '').strip()
            host_id_val = getattr(host, 'pk', None) or getattr(host, 'id', None) or ''
            safe_instance_name = re.sub(r'[^A-Za-z0-9_.-]+', '_', instance_name) or str(host_id_val)
            inventory_name = f'host_{safe_instance_name}'
            if inventory_name in used_inventory_names:
                inventory_name = f'{inventory_name}_{host_id_val}'
            used_inventory_names.add(inventory_name)
            inventory_lines.append(f'{inventory_name} ansible_host={host.ip} ansible_user=root ansible_port=22')
        inventory_lines.extend([
            '', '[all:vars]', f'ansible_ssh_private_key_file={key_path}',
            f"ansible_ssh_common_args='-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile={known_hosts_path}'",
        ])
        inventory_path.write_text('\n'.join(inventory_lines) + '\n', encoding='utf-8')
        command = [
            get_ansible_playbook_command(),
            '-i', str(inventory_path),
            '--forks', str(effective_concurrency),
            str(playbook_path),
        ]
        if extra_vars and isinstance(extra_vars, dict):
            command.extend(['--extra-vars', json.dumps(extra_vars, ensure_ascii=False)])
        run_user = str(run_as_user or '').strip()
        if run_user and run_user != 'root':
            command.extend(['--become', '--become-user', run_user])
        try:
            completed = subprocess.run(
                command,
                cwd=work_dir,
                text=True,
                capture_output=True,
                timeout=int(timeout_seconds or 600),
                check=False,
            )
            stdout, stderr, return_code = completed.stdout, completed.stderr, completed.returncode
        except subprocess.TimeoutExpired as exc:
            timeout_stdout = exc.stdout.decode('utf-8', errors='replace') if isinstance(exc.stdout, bytes) else str(exc.stdout or '')
            timeout_stderr = exc.stderr.decode('utf-8', errors='replace') if isinstance(exc.stderr, bytes) else str(exc.stderr or '')
            stdout, stderr, return_code = timeout_stdout, timeout_stderr + '\nPlaybook execution timed out.', 124

    failed_count = len(failures) + (0 if return_code == 0 else len(ready))
    if return_code == 0 and failed_count == 0:
        summary_message = 'Playbook executed successfully by backend through platform SSH key'
    elif return_code != 0:
        summary_message = extract_ansible_error_summary(stdout, stderr)
    else:
        summary_message = 'Playbook executed with partial target failures'

    summary = {
        'message': summary_message,
        'total': total_requested,
        'success': len(ready) if return_code == 0 else 0,
        'failed': failed_count,
        'execution_mode': 'backend_ansible',
        'forks': effective_concurrency,
        'rc': return_code,
    }
    combined_output = stdout + (f'\n[stderr]\n{stderr}' if stderr else '')
    return failed_count == 0, summary, combined_output, failures, ready


def execute_playbook_job(job: Any, persist_target_logs: bool = True) -> tuple[bool, dict, str]:
    """Run one multi-host playbook from backend after agents install the controller key."""
    snapshot = job.inventory_snapshot if isinstance(job.inventory_snapshot, dict) else {}
    requested = [item for item in snapshot.get('hosts', []) if isinstance(item, dict)]
    task = getattr(job, 'task', None)
    concurrency = max(1, min(int(getattr(task, 'execution_concurrency', 10) or 10), 100))
    timeout_seconds = int(getattr(task, 'execution_timeout_seconds', 600) or 600)

    host_ids = [int(item['host_id']) for item in requested if str(item.get('host_id') or '').isdigit()]
    host_map = {
        int(getattr(host, 'pk', 0) or 0): host
        for host in Host.objects.filter(id__in=host_ids)
    }
    hosts_list: list[Host] = []
    for item in requested:
        host_id = int(item['host_id']) if str(item.get('host_id') or '').isdigit() else None
        host = host_map.get(host_id) if host_id is not None else None
        if host is not None:
            hosts_list.append(host)

    success, summary, combined_output, failures, ready = run_ansible_playbook_on_hosts(
        hosts=hosts_list,
        template_content=str(getattr(job, 'template_content_snapshot', '') or ''),
        extra_vars=job.extra_vars if isinstance(job.extra_vars, dict) else {},
        run_as_user=str(getattr(job, 'run_as_user_snapshot', '') or '').strip() or 'root',
        concurrency=concurrency,
        timeout_seconds=timeout_seconds,
    )

    if persist_target_logs:
        for host, host_id, error in failures:
            AutomationExecutionTargetLog.objects.create(
                job=job,
                host=host,
                host_id_snapshot=host_id,
                host_name_snapshot=str(getattr(host, 'instance_name', '') or ''),
                host_ip_snapshot=str(getattr(host, 'ip', '') or ''),
                status='failed',
                error_message=error,
            )
        status = 'success' if summary.get('rc', 0) == 0 else 'failed'
        for host, _ in ready:
            AutomationExecutionTargetLog.objects.create(
                job=job,
                host=host,
                host_id_snapshot=getattr(host, 'pk', None) or getattr(host, 'id', None),
                host_name_snapshot=str(host.instance_name or ''),
                host_ip_snapshot=str(host.ip or ''),
                status=status,
                stdout=combined_output,
                exit_code=int(summary.get('rc', 0)),
            )

    return success, summary, combined_output