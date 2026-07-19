"""
Shell script execution engine for automation jobs.
Handles SSH-based script execution with parameter substitution and output capture.
"""
from __future__ import annotations

import asyncio
import re
import shlex
from typing import Any

from asgiref.sync import sync_to_async
from django.utils import timezone

from assets.models import Credential, Host

from .models import AnsibleExecutionJob, ShellScriptTemplate


def inject_shell_env_exports(content: str, env_vars: dict[str, Any]) -> str:
    """Inject env vars as export statements before shell script execution."""
    if not env_vars:
        return content

    export_lines = []
    for key, value in env_vars.items():
        normalized_key = str(key or '').strip()
        if not re.match(r'^[A-Za-z_][A-Za-z0-9_]*$', normalized_key):
            continue
        if isinstance(value, (dict, list)):
            continue

        export_lines.append(f'export {normalized_key}={shlex.quote(str(value))}')

    if not export_lines:
        return content

    return '\n'.join(export_lines) + '\n' + content


def _append_script_output_lines(
    job: AnsibleExecutionJob,
    text_chunk: str,
    stream_name: str = 'stdout',
    host_label: str = '-',
) -> None:
    """Append formatted output lines to job output."""
    if not text_chunk:
        return

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


def _normalize_text_chunk(text_chunk: Any) -> str:
    if text_chunk is None:
        return ''
    if isinstance(text_chunk, bytes):
        return text_chunk.decode('utf-8', errors='replace')
    return str(text_chunk)


def _append_script_output_lines_by_job_id(
    job_id: int,
    text_chunk: str,
    stream_name: str = 'stdout',
    host_label: str = '-',
) -> None:
    job = AnsibleExecutionJob.objects.filter(id=job_id).first()
    if not job:
        return
    _append_script_output_lines(job, text_chunk, stream_name, host_label)


async def _append_script_output_lines_async(
    job: AnsibleExecutionJob,
    text_chunk: Any,
    stream_name: str = 'stdout',
    host_label: str = '-',
) -> None:
    normalized = _normalize_text_chunk(text_chunk)
    if not normalized:
        return

    job_id = int(getattr(job, 'pk', 0) or 0)
    if job_id <= 0:
        return

    await sync_to_async(_append_script_output_lines_by_job_id, thread_sensitive=True)(
        job_id,
        normalized,
        stream_name,
        host_label,
    )


async def _execute_script_on_host_async(
    host: Host,
    credential: Credential,
    script_content: str,
    script_args: str,
    job: AnsibleExecutionJob,
    host_label: str,
) -> tuple[bool, str]:
    """
    Execute shell script on a single host via SSH.
    Returns: (success, error_message)
    """
    try:
        import asyncssh  # type: ignore[import-not-found]
    except ImportError:
        error_msg = 'asyncssh is not installed'
        await _append_script_output_lines_async(job, error_msg, 'stderr', host_label)
        return False, error_msg

    try:
        host_ip = str(host.ip or '').strip()
        port = credential.port or host.port or 22
        username = credential.username or 'root'

        if not host_ip:
            error_msg = f'Host {host_label} has no IP address'
            await _append_script_output_lines_async(job, error_msg, 'stderr', host_label)
            return False, error_msg

        connect_kwargs = {
            'known_hosts': None,  # Skip host key verification for automation
            'username': username,
            'port': port,
        }

        # Build authentication arguments explicitly; assigning password on options
        # object is not reliable in our runtime path.
        if credential.auth_type == Credential.AuthType.PASSWORD:
            connect_kwargs['password'] = credential.password or ''
        else:
            if credential.private_key:
                connect_kwargs['client_keys'] = [asyncssh.read_private_key(credential.private_key)]
            else:
                error_msg = f'No valid authentication method for {host_label}'
                await _append_script_output_lines_async(job, error_msg, 'stderr', host_label)
                return False, error_msg

        # Execute script
        async with asyncssh.connect(host_ip, **connect_kwargs) as conn:
            args = shlex.split(script_args) if script_args else []
            command = 'bash -s'
            if args:
                command = f"bash -s -- {' '.join(shlex.quote(item) for item in args)}"

            result = await conn.run(command, input=script_content, check=False)

            # Capture stdout
            if result.stdout:
                await _append_script_output_lines_async(job, result.stdout, 'stdout', host_label)

            # Capture stderr
            if result.stderr:
                await _append_script_output_lines_async(job, result.stderr, 'stderr', host_label)

            # Check exit code
            exit_code = result.exit_status or 0
            success = exit_code == 0

            if not success:
                error_msg = f'Script failed with exit code {exit_code}'
                await _append_script_output_lines_async(job, error_msg, 'stderr', host_label)

            return success, '' if success else 'Non-zero exit code'

    except asyncssh.HostKeyNotVerifiable:
        error_msg = f'Host key verification failed for {host_label}'
        await _append_script_output_lines_async(job, error_msg, 'stderr', host_label)
        return False, error_msg
    except asyncssh.PermissionDenied:
        error_msg = f'Authentication failed for {host_label}'
        await _append_script_output_lines_async(job, error_msg, 'stderr', host_label)
        return False, error_msg
    except asyncssh.ChannelOpenError as e:
        error_msg = f'Failed to open channel on {host_label}: {e}'
        await _append_script_output_lines_async(job, error_msg, 'stderr', host_label)
        return False, error_msg
    except (asyncssh.Error, OSError) as e:
        error_msg = f'SSH connection error on {host_label}: {e}'
        await _append_script_output_lines_async(job, error_msg, 'stderr', host_label)
        return False, error_msg
    except Exception as e:
        error_msg = f'Unexpected error executing script on {host_label}: {e}'
        await _append_script_output_lines_async(job, error_msg, 'stderr', host_label)
        return False, error_msg


async def _execute_script_on_hosts_async(
    job: AnsibleExecutionJob,
    contexts: list[tuple[Host, Credential]],
    script_content: str,
    script_args: str,
) -> tuple[int, int, int, str]:
    """
    Execute shell script hosts in order. Fail fast on first host failure.
    Returns: (success_count, failed_count, skipped_count, stop_reason)
    """
    success_count = 0
    failed_count = 0
    skipped_count = 0
    stop_reason = ''

    for idx, (host, credential) in enumerate(contexts):
        host_name = str(host.instance_name or '').strip()
        host_ip = str(host.ip or '').strip()
        host_label = f'{host_name}({host_ip})' if host_name and host_ip else (host_name or host_ip or '-')

        result = await _execute_script_on_host_async(
            host=host,
            credential=credential,
            script_content=script_content,
            script_args=script_args,
            job=job,
            host_label=host_label,
        )
        success, err_msg = result
        if success:
            success_count += 1
            continue

        # Fail-fast: stop remaining hosts once one host fails.
        failed_count = 1
        remaining_contexts = contexts[idx + 1:]
        skipped_count = len(remaining_contexts)
        stop_reason = str(err_msg or 'Host execution failed')

        for skipped_host, _ in remaining_contexts:
            skipped_name = str(skipped_host.instance_name or '').strip()
            skipped_ip = str(skipped_host.ip or '').strip()
            skipped_label = (
                f'{skipped_name}({skipped_ip})'
                if skipped_name and skipped_ip
                else (skipped_name or skipped_ip or '-')
            )
            await _append_script_output_lines_async(
                job,
                'Skipped due to previous host failure (fail-fast mode)',
                'stderr',
                skipped_label,
            )
        break

    return success_count, failed_count, skipped_count, stop_reason


def execute_shell_script_job(job: AnsibleExecutionJob) -> tuple[bool, int]:
    """
    Execute a shell script job on target hosts.
    Returns: (overall_success, return_code)
    """
    script_content = (job.template_content_snapshot or '').strip()

    if not script_content:
        error_msg = 'Script content snapshot is empty'
        _append_script_output_lines(job, error_msg, 'stderr')
        return False, 1

    # Prepare target contexts
    snapshot_hosts = job.inventory_snapshot.get('hosts', []) if isinstance(job.inventory_snapshot, dict) else []
    host_ids = [item.get('host_id') for item in snapshot_hosts if isinstance(item, dict) and item.get('host_id')]

    if not host_ids:
        error_msg = 'No target hosts found'
        _append_script_output_lines(job, error_msg, 'stderr')
        return False, 1

    from assets.models import Host

    host_map = {h.id: h for h in Host.objects.filter(id__in=host_ids)}
    contexts = []

    for host_id in host_ids:
        host = host_map.get(host_id)
        if not host:
            error_msg = f'Host {host_id} not found'
            _append_script_output_lines(job, error_msg, 'stderr')
            continue

        # Get default credential
        from assets.models import HostCredential

        host_cred = HostCredential.objects.filter(host=host, is_default=True).select_related('credential').first()
        if not host_cred or not host_cred.credential:
            error_msg = f'No default credential for host {host.instance_name}'
            _append_script_output_lines(job, error_msg, 'stderr')
            continue

        contexts.append((host, host_cred.credential))

    if not contexts:
        error_msg = 'No executable targets after filtering'
        _append_script_output_lines(job, error_msg, 'stderr')
        return False, 1

    shell_parameters = str(job.shell_parameters or '').strip()
    shell_env_vars = job.shell_env_vars if isinstance(job.shell_env_vars, dict) else {}

    # Inject env vars for the script process environment.
    if shell_env_vars:
        script_content = inject_shell_env_exports(script_content, shell_env_vars)

    # Execute scripts in host order; stop at first failure.
    try:
        success_count, failed_count, skipped_count, stop_reason = asyncio.run(
            _execute_script_on_hosts_async(job, contexts, script_content, shell_parameters)
        )
        if failed_count > 0 and stop_reason:
            _append_script_output_lines(job, f'Execution stopped: {stop_reason}', 'stderr')

        overall_success = failed_count == 0
        return_code = 0 if overall_success else 1
        summary = {
            'message': 'Shell script execution finished',
            'total': len(contexts),
            'success': success_count,
            'failed': failed_count,
            'skipped': skipped_count,
            'rc': return_code,
            'mode': 'serial_fail_fast',
        }
        existing_summary = job.result_summary if isinstance(job.result_summary, dict) else {}
        existing_summary.update(summary)
        job.result_summary = existing_summary
        job.save(update_fields=['result_summary'])
        return overall_success, return_code
    except Exception as e:
        error_msg = f'Script execution failed: {e}'
        _append_script_output_lines(job, error_msg, 'stderr')
        return False, 1
