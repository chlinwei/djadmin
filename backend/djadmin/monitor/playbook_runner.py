from __future__ import annotations

import logging
from typing import Any

from django.conf import settings
from django.db import DatabaseError, close_old_connections
from django.utils import timezone

from automation.executor_playbook import run_ansible_playbook_on_hosts
from monitor.models import MonitorTargetInstallHistory

logger = logging.getLogger(__name__)


def _resolve_monitor_timeout_seconds() -> int:
    """监控安装/卸载执行超时秒数（默认 600 秒，可在 settings 覆盖）。"""
    raw_value = getattr(settings, 'MONITOR_EXECUTION_TIMEOUT_SECONDS', 600)
    try:
        timeout_seconds = int(raw_value)
    except (TypeError, ValueError):
        timeout_seconds = 600
    return max(timeout_seconds, 30)


def _resolve_monitor_pending_stale_seconds(timeout_seconds: int) -> int:
    """pending 判定为“卡死”的阈值：执行超时 + 90 秒缓冲。"""
    raw_value = getattr(settings, 'MONITOR_PENDING_STALE_SECONDS', None)
    if raw_value is None:
        return int(timeout_seconds) + 90
    try:
        stale_seconds = int(raw_value)
    except (TypeError, ValueError):
        stale_seconds = int(timeout_seconds) + 90
    return max(stale_seconds, int(timeout_seconds))


def _split_monitor_output_snapshots(output_text: str) -> dict[str, str]:
    """拆分 Ansible 混合输出为 stdout/stderr/error 块，便于前端独立 Tab 展示。"""
    if not output_text:
        return {'stdout': '', 'stderr': '', 'error': ''}

    stdout_parts: list[str] = []
    stderr_parts: list[str] = []
    error_parts: list[str] = []
    current_section = 'stdout'
    section_lines: list[str] = []

    def _flush_section() -> None:
        nonlocal section_lines
        if not section_lines:
            return
        content = '\n'.join(section_lines).strip()
        section_lines = []
        if not content:
            return
        if current_section == 'stderr':
            stderr_parts.append(content)
        elif current_section == 'error':
            error_parts.append(content)
        else:
            stdout_parts.append(content)

    for line in output_text.splitlines():
        trimmed = line.strip()
        if trimmed == '[stderr]':
            _flush_section()
            current_section = 'stderr'
            continue
        if trimmed == '[error]':
            _flush_section()
            current_section = 'error'
            continue
        section_lines.append(line)

    _flush_section()
    return {
        'stdout': '\n\n'.join(stdout_parts),
        'stderr': '\n\n'.join(stderr_parts),
        'error': '\n\n'.join(error_parts),
    }


def _expire_stale_monitor_pending(target: Any, action: str, stale_seconds: int) -> bool:
    """若最新 pending/running 历史已超时，自动转 failed 并允许本次重新下发。"""
    latest_history = MonitorTargetInstallHistory.objects.filter(
        target_id=getattr(target, 'id', None),
        action=action,
        status__in=[
            MonitorTargetInstallHistory.Status.PENDING,
            MonitorTargetInstallHistory.Status.RUNNING,
        ],
    ).order_by('-id').first()

    if latest_history is None:
        return False

    reference_time = latest_history.start_time or latest_history.create_time
    if reference_time is None:
        return False

    elapsed_seconds = (timezone.now() - reference_time).total_seconds()
    if elapsed_seconds < float(stale_seconds):
        return False

    timeout_message = f'执行超时（超过 {int(stale_seconds)} 秒），已标记失败，请重新下发'
    latest_history.status = MonitorTargetInstallHistory.Status.FAILED
    latest_history.summary_message = timeout_message
    latest_history.error_message_snapshot = timeout_message
    latest_history.end_time = timezone.now()
    start_ts = latest_history.start_time
    end_ts = latest_history.end_time
    if start_ts is not None and end_ts is not None:
        latest_history.duration_seconds = (end_ts - start_ts).total_seconds()
    latest_history.save(update_fields=[
        'status', 'summary_message', 'error_message_snapshot', 'end_time', 'duration_seconds', 'update_time',
    ])

    target.install_status = target.InstallStatus.FAILED
    target.install_message = timeout_message
    target.save(update_fields=['install_status', 'install_message', 'update_time'])
    return True


def run_monitor_playbook_and_update_history(
    *,
    target: Any,
    host: Any,
    history: MonitorTargetInstallHistory,
    template_content: str,
    extra_vars: dict[str, Any] | None,
    work_directory: str = '/tmp',
    timeout_seconds: int = 600,
) -> None:
    """通过 backend Ansible 执行监控组件/Fluent Bit 安装或卸载，并原子回写执行状态与历史。"""
    close_old_connections()
    history_id = int(getattr(history, 'id', 0) or 0)
    target_id = int(getattr(target, 'id', 0) or 0)

    def _safe_history_save(fields: list[str]) -> bool:
        if history_id <= 0:
            return False
        try:
            history.save(update_fields=fields)
            return True
        except DatabaseError:
            logger.debug('Skip monitor history save: history row disappeared (id=%s)', history_id)
            return False

    def _safe_target_save(fields: list[str]) -> bool:
        if target_id <= 0:
            return False
        try:
            target.save(update_fields=fields)
            return True
        except DatabaseError:
            logger.debug('Skip monitor target save: target row disappeared (id=%s)', target_id)
            return False

    def _is_history_cancelled() -> bool:
        if history_id <= 0:
            return False
        current_status = MonitorTargetInstallHistory.objects.filter(id=history_id).values_list('status', flat=True).first()
        return current_status == MonitorTargetInstallHistory.Status.CANCELLED

    started_at = timezone.now()
    if _is_history_cancelled():
        logger.info('Monitor history already cancelled before execution start, skip run (history_id=%s)', history_id)
        return

    history.status = MonitorTargetInstallHistory.Status.RUNNING
    history.start_time = started_at
    if not _safe_history_save(['status', 'start_time', 'update_time']):
        return

    try:
        success, summary, output_text, _failures, _ready = run_ansible_playbook_on_hosts(
            hosts=[host],
            template_content=str(template_content or ''),
            extra_vars=extra_vars if isinstance(extra_vars, dict) else {},
            run_as_user='root',
            concurrency=1,
            timeout_seconds=int(timeout_seconds),
        )
        finished_at = timezone.now()
        duration_seconds = (finished_at - started_at).total_seconds()
        snapshots = _split_monitor_output_snapshots(output_text)
        message_text = str((summary or {}).get('message', '') or '')

        if _is_history_cancelled():
            logger.info('Monitor history cancelled during execution, skip final write-back (history_id=%s)', history_id)
            return

        if success:
            target.install_status = target.InstallStatus.SUCCESS
            target.install_message = '执行成功'
            target.retry_count = 0
            _safe_target_save(['install_status', 'install_message', 'retry_count', 'update_time'])
            history.status = MonitorTargetInstallHistory.Status.SUCCESS
            history.summary_message = target.install_message
        else:
            target.install_status = target.InstallStatus.FAILED
            if getattr(target, 'last_dispatch_manual', False):
                target.install_message = (
                    f'执行失败：{message_text}，人工重试失败，如需再次尝试请再次点击“重试”'
                )
            else:
                target.install_message = f'执行失败：{message_text}，需人工重试'
            _safe_target_save(['install_status', 'install_message', 'update_time'])
            history.status = MonitorTargetInstallHistory.Status.FAILED
            history.summary_message = target.install_message

        history.stdout_snapshot = snapshots['stdout']
        history.stderr_snapshot = snapshots['stderr']
        history.error_message_snapshot = snapshots['error']
        history.result_summary_snapshot = summary if isinstance(summary, dict) else {}
        history.end_time = finished_at
        history.duration_seconds = duration_seconds
        _safe_history_save([
            'status', 'summary_message', 'stdout_snapshot', 'stderr_snapshot', 'error_message_snapshot',
            'result_summary_snapshot', 'end_time', 'duration_seconds', 'update_time',
        ])
    except Exception as exc:
        finished_at = timezone.now()
        duration_seconds = (finished_at - started_at).total_seconds()
        target.install_status = target.InstallStatus.FAILED
        target.install_message = f'执行失败：{exc}'
        _safe_target_save(['install_status', 'install_message', 'update_time'])
        history.status = MonitorTargetInstallHistory.Status.FAILED
        history.summary_message = target.install_message
        history.error_message_snapshot = str(exc)
        history.result_summary_snapshot = {'message': str(exc)}
        history.end_time = finished_at
        history.duration_seconds = duration_seconds
        _safe_history_save([
            'status', 'summary_message', 'error_message_snapshot', 'result_summary_snapshot',
            'end_time', 'duration_seconds', 'update_time',
        ])
    finally:
        close_old_connections()
