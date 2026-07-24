from __future__ import annotations

import uuid
from typing import Any

from assets.grpc_transfer.client import AgentChannelClient, AgentGrpcTransferError
from assets.models import AgentJob, Host
from .models import AutomationExecutionTargetLog, AutomationExecutionJob


def _execute_automation_task_via_agent_grpc(agent_id: str, job_id: str, params: dict[str, Any], timeout_seconds: int) -> dict[str, Any]:
    """通过 agent 长连接 gRPC 通道下发一次自动化任务并同步等待结果。

    取代旧的 backend 直连 host.ip:19090 HTTP 执行路径：改为按 agent_id 在
    AgentChannel gRPC 注册表里查活跃 Session 下发（agent 主动拨入，天然穿透 NAT）。
    agent 未在线/未建立通道时，AgentChannelClient 构造即抛 AgentGrpcTransferError，
    与旧路径一样由上层逐主机 try/except 捕获并记为该主机失败。
    """
    client = AgentChannelClient(agent_id)
    return client.execute_automation(
        job_id=job_id,
        params=params,
        timeout_seconds=int(timeout_seconds),
        task_type='custom',
        action='run_automation_task',
    )


def execute_job_via_agent_grpc(
    *,
    automation_execution_job_id: int,
    automation_task_id: int,
    template_content: str,
    template_type: str,
    hosts: list[dict[str, Any]],
    shell_parameters: str,
    shell_env_vars: dict[str, Any],
    extra_vars: dict[str, Any],
    run_as_user: str,
    run_as_group: str,
    work_directory: str,
    timeout_seconds: int,
) -> tuple[bool, dict[str, Any], str]:
    target_host_ids: list[int] = []
    for item in hosts:
        if not isinstance(item, dict):
            continue
        host_id_text = str(item.get('host_id') or '').strip()
        if host_id_text.isdigit():
            target_host_ids.append(int(host_id_text))

    host_map = {
        int(getattr(row, 'pk', 0) or 0): row
        for row in Host.objects.filter(id__in=target_host_ids)
        if int(getattr(row, 'pk', 0) or 0) > 0
    }

    created_count = 0
    success_count = 0
    failed_count = 0
    failed_rows = []
    output_chunks = []

    is_shell_task = template_type == 'shell_script'
    job = AutomationExecutionJob.objects.filter(id=int(automation_execution_job_id)).first()

    def _append_host_log(
        *,
        host_obj: Host | None,
        host_id_snapshot: int | None,
        host_name_snapshot: str,
        host_ip_snapshot: str,
        agent_job_id: str,
        status: str,
        stdout: str,
        stderr: str,
        error_message: str,
        exit_code: int | None,
        result_data: dict[str, Any],
    ) -> None:
        if job is None:
            return
        AutomationExecutionTargetLog.objects.create(
            job=job,
            host=host_obj,
            host_id_snapshot=host_id_snapshot,
            host_name_snapshot=host_name_snapshot,
            host_ip_snapshot=host_ip_snapshot,
            agent_job_id=agent_job_id,
            status=status,
            stdout=stdout,
            stderr=stderr,
            error_message=error_message,
            exit_code=exit_code,
            result_data=result_data,
        )

    for item in hosts:
        if not isinstance(item, dict):
            continue
        host_id_raw = item.get('host_id')
        host_id_text = str(host_id_raw or '')
        if not host_id_text.isdigit():
            failed_count += 1
            host_id_snapshot = int(host_id_text) if host_id_text.isdigit() else None
            _append_host_log(
                host_obj=None,
                host_id_snapshot=host_id_snapshot,
                host_name_snapshot='',
                host_ip_snapshot='',
                agent_job_id='',
                status=AgentJob.JobStatus.FAILED,
                stdout='',
                stderr='',
                error_message='invalid host_id',
                exit_code=None,
                result_data={},
            )
            failed_rows.append({'host_id': host_id_raw, 'error': 'invalid host_id'})
            continue

        host_id = int(host_id_text)
        host = host_map.get(host_id)
        if host is None:
            failed_count += 1
            _append_host_log(
                host_obj=None,
                host_id_snapshot=host_id,
                host_name_snapshot='',
                host_ip_snapshot='',
                agent_job_id='',
                status=AgentJob.JobStatus.FAILED,
                stdout='',
                stderr='',
                error_message='host not found',
                exit_code=None,
                result_data={},
            )
            failed_rows.append({'host_id': host_id, 'error': 'host not found'})
            continue

        agent_id = str(host.instance_name or '').strip()
        if agent_id == '':
            failed_count += 1
            _append_host_log(
                host_obj=host,
                host_id_snapshot=host_id,
                host_name_snapshot=str(host.instance_name or ''),
                host_ip_snapshot=str(host.ip or ''),
                agent_job_id='',
                status=AgentJob.JobStatus.FAILED,
                stdout='',
                stderr='',
                error_message='host has empty instance_name',
                exit_code=None,
                result_data={},
            )
            failed_rows.append({'host_id': host_id, 'error': 'host has empty instance_name'})
            continue

        current_job_id = f'run_automation_task-{uuid.uuid4().hex[:16]}'
        current_params = {
            'template_type': template_type,
            'template_content': template_content,
            'shell_parameters': shell_parameters if is_shell_task else '',
            'env_vars': shell_env_vars if is_shell_task else {},
            'extra_vars': extra_vars if not is_shell_task else {},
            # 执行身份/工作目录：dj-agent 以 root 运行，收到后通过 setuid/setgid 降权到 run_as_user/run_as_group
            # 执行（见 dj_agent automation.go resolveRunAsCredential），不再使用 ansible become 机制。
            'run_as_user': run_as_user,
            'run_as_group': run_as_group,
            'work_dir': work_directory,
            'automation_execution_job_id': int(automation_execution_job_id),
            'automation_task_id': int(automation_task_id),
            'host_id': int(host_id),
            'host_ip': str(host.ip or ''),
        }

        created_count += 1
        try:
            exec_result = _execute_automation_task_via_agent_grpc(
                agent_id=agent_id,
                job_id=current_job_id,
                params=current_params,
                timeout_seconds=int(timeout_seconds),
            )
            current_status = str(exec_result.get('status') or '').strip().lower()
            stdout_text = str(exec_result.get('stdout') or '')
            stderr_text = str(exec_result.get('stderr') or '')
            error_text = str(exec_result.get('error_message') or '')
            exit_code_raw = exec_result.get('exit_code')
            exit_code_text = str(exit_code_raw or '').strip()
            exit_code = int(exit_code_text) if exit_code_text.lstrip('-').isdigit() else None
            result_data_raw = exec_result.get('result_data')
            result_data = result_data_raw if isinstance(result_data_raw, dict) else {}

            if current_status == AgentJob.JobStatus.SUCCESS:
                success_count += 1
            else:
                failed_count += 1
                failed_rows.append({
                    'host_id': host_id,
                    'error': error_text or f'agent status={current_status or "unknown"}',
                })

            # 按主机维度持久化执行明细，供运行记录中心“详细日志”查看。
            _append_host_log(
                host_obj=host,
                host_id_snapshot=host_id,
                host_name_snapshot=str(host.instance_name or ''),
                host_ip_snapshot=str(host.ip or ''),
                agent_job_id=current_job_id,
                status=current_status or AgentJob.JobStatus.FAILED,
                stdout=stdout_text,
                stderr=stderr_text,
                error_message=error_text,
                exit_code=exit_code,
                result_data=result_data,
            )

            output_chunks.append(
                f"\n\n===== Agent Host #{host_id} ({host.ip or '-'}) | status={current_status or 'unknown'} | job={current_job_id} =====\n"
            )
            if stdout_text:
                output_chunks.append(stdout_text.rstrip('\n') + '\n')
            if stderr_text:
                output_chunks.append('[stderr]\n' + stderr_text.rstrip('\n') + '\n')
            if error_text:
                output_chunks.append('[error]\n' + error_text.rstrip('\n') + '\n')
        except Exception as exc:
            failed_count += 1
            _append_host_log(
                host_obj=host,
                host_id_snapshot=host_id,
                host_name_snapshot=str(host.instance_name or ''),
                host_ip_snapshot=str(host.ip or ''),
                agent_job_id=current_job_id,
                status=AgentJob.JobStatus.FAILED,
                stdout='',
                stderr='',
                error_message=str(exc),
                exit_code=None,
                result_data={},
            )
            failed_rows.append({'host_id': host_id, 'error': str(exc)})
            output_chunks.append(
                f"\n\n===== Agent Host #{host_id} ({host.ip or '-'}) | status=failed | job={current_job_id} =====\n"
            )
            output_chunks.append('[error]\n' + str(exc).rstrip('\n') + '\n')

    success = created_count > 0 and failed_count == 0
    summary = {
        'message': 'Job executed synchronously via agent grpc' if created_count > 0 else 'Failed to dispatch any agent task',
        'created_count': created_count,
        'success_count': success_count,
        'failed_count': failed_count,
        'failed_rows': failed_rows,
        'execution_mode': 'agent_grpc_sync',
    }
    return success, summary, ''.join(output_chunks)