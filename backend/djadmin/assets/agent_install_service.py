from __future__ import annotations

import json
import os
import re
import selectors
import signal
import subprocess
import tempfile
import time
from pathlib import Path
from urllib.parse import urlsplit

from django.conf import settings
from django.db import close_old_connections
from django.db.models import Q
from django.utils import timezone

from .credential_crypto import decrypt_secret
from .grpc_transfer.registry import REGISTRY
from .models import AgentJob, Credential, Host
from automation.models import AutomationExecutionJob, AutomationExecutionTargetLog
from sys_config.models import SysConfig


def _terminate_process_group(process: subprocess.Popen[bytes]) -> None:
    if process.poll() is not None:
        return
    os.killpg(process.pid, signal.SIGTERM)
    try:
        process.wait(timeout=3)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait()


def _stream_process_output(
    process: subprocess.Popen[bytes],
    timeout_seconds: int,
    output_callback,
) -> tuple[str, int]:
    if process.stdout is None:
        raise RuntimeError('Ansible 进程未创建输出管道')

    output_chunks: list[bytes] = []
    deadline = time.monotonic() + timeout_seconds
    selector = selectors.DefaultSelector()
    selector.register(process.stdout, selectors.EVENT_READ)
    try:
        while selector.get_map():
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                _terminate_process_group(process)
                output = b''.join(output_chunks).decode('utf-8', errors='replace')
                raise subprocess.TimeoutExpired(process.args, timeout_seconds, output=output)

            events = selector.select(timeout=min(1.0, remaining))
            if not events:
                output_callback(b''.join(output_chunks).decode('utf-8', errors='replace'))
            for key, _ in events:
                chunk = os.read(key.fd, 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                    continue
                output_chunks.append(chunk)
                output_callback(b''.join(output_chunks).decode('utf-8', errors='replace'))

        remaining = deadline - time.monotonic()
        if remaining <= 0:
            _terminate_process_group(process)
            output = b''.join(output_chunks).decode('utf-8', errors='replace')
            raise subprocess.TimeoutExpired(process.args, timeout_seconds, output=output)
        returncode = process.wait(timeout=remaining)
    except BaseException:
        _terminate_process_group(process)
        raise
    finally:
        selector.close()

    return b''.join(output_chunks).decode('utf-8', errors='replace'), returncode


def _parse_ansible_recap(output: str) -> dict[str, int]:
    recap = {'ok': 0, 'changed': 0, 'unreachable': 0, 'failed': 0, 'skipped': 0}
    match = re.search(r'\b(?:ok|changed|unreachable|failed|skipped)=\d+', output)
    if not match:
        return recap
    line = output[match.start():].splitlines()[0]
    for key in recap:
        value = re.search(rf'\b{key}=(\d+)', line)
        if value:
            recap[key] = int(value.group(1))
    return recap


def _wait_for_agent_connection(agent_id: str, timeout_seconds: int = 10) -> bool:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if REGISTRY.is_connected(agent_id):
            return True
        time.sleep(1)
    return REGISTRY.is_connected(agent_id)


def _agent_package_path(name: str) -> Path:
    package_root = Path(settings.BASE_DIR).resolve().parents[1] / 'dj_agent'
    return package_root / 'bin' / name


def _binary_contains_marker(binary: Path, marker: bytes) -> bool:
    overlap = b''
    with binary.open('rb') as source:
        while chunk := source.read(1024 * 1024):
            data = overlap + chunk
            if marker in data:
                return True
            overlap = data[-max(len(marker) - 1, 0):]
    return False


def _validate_agent_binary(binary: Path) -> None:
    """只允许部署当前纯 gRPC Agent，避免旧 RabbitMQ 构建产物覆盖在线服务。"""
    if not binary.is_file():
        raise RuntimeError(f'Agent 二进制不存在: {binary}')
    if _binary_contains_marker(binary, b'connect rabbitmq failed'):
        raise RuntimeError(
            'Agent 二进制仍是旧 RabbitMQ 版本，请使用当前源码执行 '
            '`CGO_ENABLED=0 go build -trimpath -o bin/dj-agent ./cmd/agent` 后重试'
        )
    if not _binary_contains_marker(binary, b'DJ_AGENT_GRPC_FILE_ADDR'):
        raise RuntimeError('Agent 二进制缺少当前 gRPC 配置标记，拒绝部署未知版本')


def _agent_grpc_addr_for_host(host: Host, advertised_addr: str) -> tuple[str, bool]:
    endpoint = urlsplit(f'//{advertised_addr}')
    advertised_host = endpoint.hostname
    advertised_port = endpoint.port
    if not advertised_host or advertised_port is None:
        raise RuntimeError('“Agent gRPC 对外地址”格式错误，应为 主机:端口')

    # 只有与 djadmin 对外地址相同的目标主机才能走本机回环地址。
    is_local_host = str(host.ip).strip().lower() == advertised_host.strip().lower()
    if is_local_host:
        return f'127.0.0.1:{advertised_port}', True
    return advertised_addr, False


def _inventory_for_host(host: Host, credential: Credential, key_path: Path | None) -> dict:
    variables = {
        'ansible_host': str(host.ip),
        'ansible_user': str(credential.username or 'root'),
        'ansible_port': int(credential.port or 22),
    }
    if key_path is not None:
        variables['ansible_ssh_private_key_file'] = str(key_path)
    else:
        variables['ansible_password'] = str(decrypt_secret(credential.password) or '')
    return {
        'all': {'hosts': {'target': variables}},
    }


def run_agent_install_job(job_id: int, host_id: int, credential_id: int, automation_job_id: int | None = None) -> None:
    close_old_connections()
    try:
        job = AgentJob.objects.get(id=job_id)
        host = Host.objects.get(id=host_id)
        credential = Credential.objects.get(id=credential_id)
        job.status = AgentJob.JobStatus.RUNNING
        job.picked_at = timezone.now()
        job.save(update_fields=['status', 'picked_at', 'update_time'])
        if automation_job_id:
            AutomationExecutionTargetLog.objects.filter(
                job_id=automation_job_id,
                agent_job_id=job.job_id,
            ).update(status=AutomationExecutionTargetLog.Status.RUNNING)

        playbook = Path(settings.BASE_DIR) / 'assets' / 'agent_install.yml'
        binary = _agent_package_path('dj-agent')
        _validate_agent_binary(binary)
        if not playbook.is_file():
            raise RuntimeError(f'Ansible Playbook 不存在: {playbook}')
        if credential.auth_type == Credential.AuthType.PASSWORD and not credential.password:
            raise RuntimeError('SSH 密码凭证为空')
        if credential.auth_type == Credential.AuthType.SSH_KEY and not credential.private_key:
            raise RuntimeError('SSH Key 凭证为空')

        agent_id = str(host.agent_id or '').strip() or f'host-{host.pk}'
        advertised_grpc_addr = str(SysConfig.objects.filter(
            key='sys.assets.agent.grpc_advertise_addr',
        ).values_list('value', flat=True).first() or '').strip()
        if not advertised_grpc_addr:
            raise RuntimeError('未配置“Agent gRPC 对外地址”，请先在系统参数中填写')
        grpc_addr, is_local_host = _agent_grpc_addr_for_host(host, advertised_grpc_addr)
        with tempfile.TemporaryDirectory(prefix='djadmin-agent-install-') as work_dir:
            work_path = Path(work_dir)
            inventory_path = work_path / 'inventory.json'
            inventory_path.write_text(
                json.dumps(_inventory_for_host(host, credential, None), ensure_ascii=False),
                encoding='utf-8',
            )
            key_path = None
            if credential.auth_type == Credential.AuthType.SSH_KEY:
                key_path = work_path / 'ssh_key'
                key_path.write_text(str(decrypt_secret(credential.private_key) or ''), encoding='utf-8')
                os.chmod(key_path, 0o600)
                inventory_path.write_text(
                    json.dumps(_inventory_for_host(host, credential, key_path), ensure_ascii=False),
                    encoding='utf-8',
                )

            command = [
                'ansible-playbook', '-i', str(inventory_path), '--timeout', '10',
                '-e', f'dj_agent_binary_source={binary}',
                '-e', f'dj_agent_id={agent_id}',
                '-e', f'dj_agent_grpc_addr={grpc_addr}',
                '-e', f'dj_agent_is_local={str(is_local_host).lower()}',
                str(playbook),
            ]
            process = subprocess.Popen(
                command,
                cwd=str(work_path),
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                start_new_session=True,
            )

            def persist_output(live_output: str) -> None:
                AgentJob.objects.filter(id=job.pk).update(stdout=live_output, update_time=timezone.now())
                if automation_job_id:
                    AutomationExecutionTargetLog.objects.filter(
                        job_id=automation_job_id,
                        agent_job_id=job.job_id,
                    ).update(stdout=live_output)

            completed_stdout, completed_returncode = _stream_process_output(
                process,
                int(job.timeout_seconds or 60),
                persist_output,
            )

        job.stdout = completed_stdout
        job.stderr = ''
        job.exit_code = completed_returncode
        recap = _parse_ansible_recap(job.stdout)
        has_recap_failure = recap['failed'] > 0 or recap['unreachable'] > 0
        job.status = AgentJob.JobStatus.SUCCESS if completed_returncode == 0 and not has_recap_failure else AgentJob.JobStatus.FAILED
        job.result_data = {
            'host_id': host.pk,
            'agent_id': agent_id,
            'operation': job.params.get('operation', 'install'),
            'ansible_recap': recap,
        }
        job.error_message = '' if job.status == AgentJob.JobStatus.SUCCESS else 'Ansible 安装失败'
        agent_connected = False
        if job.status == AgentJob.JobStatus.SUCCESS:
            agent_connected = _wait_for_agent_connection(agent_id)
            if not agent_connected:
                job.status = AgentJob.JobStatus.FAILED
                job.error_message = 'Ansible 安装完成，但 Agent 未连接到后端 gRPC 服务'
        job.result_data['agent_connected'] = agent_connected
        job.finished_at = timezone.now()
        job.save(update_fields=['status', 'stdout', 'stderr', 'exit_code', 'result_data', 'error_message', 'finished_at', 'update_time'])
        if automation_job_id:
            AutomationExecutionTargetLog.objects.filter(
                job_id=automation_job_id,
                agent_job_id=job.job_id,
            ).update(
                status=(AutomationExecutionTargetLog.Status.SUCCESS if job.status == AgentJob.JobStatus.SUCCESS else AutomationExecutionTargetLog.Status.FAILED),
                exit_code=job.exit_code,
                stdout=job.stdout,
                stderr=job.stderr,
                error_message=job.error_message,
                result_data=job.result_data,
            )
            _refresh_automation_job(automation_job_id)
        if completed_returncode == 0 and not host.agent_id:
            Host.objects.filter(id=host.pk).filter(Q(agent_id__isnull=True) | Q(agent_id='')).update(
                agent_id=agent_id,
                update_time=timezone.now(),
            )
    except subprocess.TimeoutExpired as exc:
        timeout_stdout = str(exc.stdout or '')
        AgentJob.objects.filter(id=job_id).update(
            status=AgentJob.JobStatus.TIMEOUT,
            error_message='Ansible Agent 安装任务超时',
            stdout=timeout_stdout,
            stderr=str(exc.stderr or ''),
            exit_code=124,
            finished_at=timezone.now(),
            update_time=timezone.now(),
        )
        if automation_job_id:
            AutomationExecutionTargetLog.objects.filter(
                job_id=automation_job_id,
                agent_job_id=job.job_id,
            ).update(
                status=AutomationExecutionTargetLog.Status.FAILED,
                stdout=timeout_stdout,
                error_message='Ansible Agent 安装任务超时',
                exit_code=124,
            )
            _refresh_automation_job(automation_job_id)
    except Exception as exc:
        AgentJob.objects.filter(id=job_id).update(
            status=AgentJob.JobStatus.FAILED,
            error_message=str(exc),
            exit_code=1,
            finished_at=timezone.now(),
            update_time=timezone.now(),
        )
        if automation_job_id:
            AutomationExecutionTargetLog.objects.filter(
                job_id=automation_job_id,
                agent_job_id=job.job_id,
            ).update(status=AutomationExecutionTargetLog.Status.FAILED, error_message=str(exc), exit_code=1)
            _refresh_automation_job(automation_job_id)
    finally:
        close_old_connections()


def _refresh_automation_job(automation_job_id: int) -> None:
    execution = AutomationExecutionJob.objects.get(id=automation_job_id)
    statuses = list(AutomationExecutionTargetLog.objects.filter(job_id=execution.pk).values_list('status', flat=True))
    if not statuses or any(status in {'pending', 'running'} for status in statuses):
        return
    now = timezone.now()
    execution.status = AutomationExecutionJob.Status.SUCCESS if all(
        status == AutomationExecutionTargetLog.Status.SUCCESS for status in statuses
    ) else AutomationExecutionJob.Status.FAILED
    execution.end_time = now
    execution.duration_seconds = (now - execution.start_time).total_seconds() if execution.start_time else None
    execution.result_summary = {
        'total': len(statuses),
        'success': statuses.count(AutomationExecutionTargetLog.Status.SUCCESS),
        'failed': statuses.count(AutomationExecutionTargetLog.Status.FAILED),
    }
    execution.save(update_fields=['status', 'end_time', 'duration_seconds', 'result_summary', 'update_time'])
