"""巡检的调度与维护逻辑：定时分发、僵尸执行兜底与执行记录清理。

分发与兜底跑在 runserver 进程内的后台线程里，不能放到 Celery：巡检要通过 agent 的
gRPC 长连接下发作业，而连接注册表只存在于 Web 进程内存中。
清理是纯数据库操作，仍由任务中心（Celery）调度。
"""
from __future__ import annotations

import logging
import os
import sys
import threading
from datetime import timedelta

from django.db import close_old_connections
from django.utils import timezone

from sys_config.models import SysConfig

from .executor import TARGET_WAIT_GRACE_SECONDS
from .models import InspectionExecution, InspectionTargetExecution, InspectionTask
from .service import InspectionRequestError, create_execution

logger = logging.getLogger(__name__)

EXECUTION_RETENTION_KEY = 'sys.inspection.executions.retention_days'
# 执行线程未把记录推进 running 的容忍窗口；超过则认为进程已重启，线程丢失。
PENDING_TIMEOUT_SECONDS = 300
# 单目标超时之外再留的整体余量，覆盖并发排队与结果落库时间。
RUNNING_EXTRA_GRACE_SECONDS = 300
SCAN_INTERVAL_SECONDS = 60

_scheduler_lock = threading.Lock()
_scheduler_started = False


def calculate_next_run_time(task, base=None):
    """按 cron 推进下次运行时间；未配置 cron 或已停用时清空，表示只允许手动触发。"""
    # 延迟导入：scheduler_manager 会反向导入本模块的清理函数，顶层导入会形成循环。
    from scheduler_manager import parse_cron_expression

    base = base or timezone.now()
    cron_text = str(task.cron_expression or '').strip()
    next_time = None
    if task.enabled and cron_text:
        due_state = parse_cron_expression(cron_text).is_due(base)
        next_seconds = float(getattr(due_state, 'next', 0) or 0)
        # is_due 恰好命中时 next 可能为 0，至少推进一分钟，避免同一分钟内重复触发。
        if next_seconds <= 0:
            next_seconds = 60.0
        next_time = base + timedelta(seconds=next_seconds)
    task.next_run_time = next_time
    task.save(update_fields=['next_run_time', 'update_time'])
    return next_time


def dispatch_scheduled_inspections():
    """扫描到期的巡检任务并触发执行。"""
    now = timezone.now()
    tasks = (
        InspectionTask.objects
        .filter(enabled=True)
        .exclude(cron_expression='')
        .select_related('group', 'logical_service')
    )
    dispatched = 0
    for task in tasks:
        if task.next_run_time is None:
            # 首次配置 cron 时只建立时间基线，不立即补跑，避免保存任务即触发一轮巡检。
            calculate_next_run_time(task, now)
            continue
        if task.next_run_time > now:
            continue
        try:
            create_execution(
                task,
                requested_username='scheduler',
                trigger_type=InspectionExecution.TriggerType.SCHEDULED,
            )
            dispatched += 1
        except InspectionRequestError as exc:
            print(f'[INSPECTION] skip scheduled task {task.name}: {exc}')
        task.last_run_time = now
        task.save(update_fields=['last_run_time', 'update_time'])
        calculate_next_run_time(task, now)
    return dispatched


def fail_stale_inspection_executions():
    """把卡死的执行记录收敛为失败，避免服务重启后记录永久停在 pending/running。"""
    now = timezone.now()
    stale_ids = []

    pending_cutoff = now - timedelta(seconds=PENDING_TIMEOUT_SECONDS)
    for execution in InspectionExecution.objects.filter(
        status=InspectionExecution.Status.PENDING,
        create_time__lt=pending_cutoff,
    ):
        execution.summary = {
            **(execution.summary or {}),
            'error': '巡检未被执行线程受理，服务可能在提交后重启',
        }
        execution.status = InspectionExecution.Status.FAILED
        execution.end_time = now
        execution.save(update_fields=['status', 'summary', 'end_time', 'update_time'])
        stale_ids.append(execution.pk)

    for execution in InspectionExecution.objects.filter(status=InspectionExecution.Status.RUNNING):
        timeout_seconds = int((execution.task_snapshot or {}).get('timeout_seconds') or 60)
        deadline = timeout_seconds + TARGET_WAIT_GRACE_SECONDS + RUNNING_EXTRA_GRACE_SECONDS
        started_at = execution.start_time or execution.create_time
        if started_at > now - timedelta(seconds=deadline):
            continue
        InspectionTargetExecution.objects.filter(
            execution=execution,
            status__in=[InspectionTargetExecution.Status.PENDING, InspectionTargetExecution.Status.RUNNING],
        ).update(status=InspectionTargetExecution.Status.FAILED, passed=False, end_time=now)
        execution.summary = {
            **(execution.summary or {}),
            'error': '巡检执行超过整体时限，可能是执行线程中断',
        }
        execution.status = InspectionExecution.Status.FAILED
        execution.end_time = now
        execution.save(update_fields=['status', 'summary', 'end_time', 'update_time'])
        stale_ids.append(execution.pk)

    if stale_ids:
        print(f'[INSPECTION] stale executions failed: {stale_ids}')
    return len(stale_ids)


def cleanup_inspection_executions():
    """按系统参数清理巡检执行记录（目标明细与结果随外键级联删除）。"""
    config, _ = SysConfig.objects.get_or_create(
        key=EXECUTION_RETENTION_KEY,
        defaults={
            'value': '90',
            'default_value': '90',
            'value_type': 'int',
            'name': '巡检执行记录保留天数',
            'description': '巡检执行记录及其检查结果在数据库中的保留天数',
            'is_readonly': False,
        },
    )
    try:
        retention_days = max(1, int(str(config.value).strip()))
    except (TypeError, ValueError):
        retention_days = 90

    cutoff = timezone.now() - timedelta(days=retention_days)
    deleted, _ = InspectionExecution.objects.filter(create_time__lt=cutoff).delete()
    print(f'[CLEANUP] inspection executions cleaned: deleted={deleted}, retention_days={retention_days}')
    return deleted


def _scheduler_loop():
    while True:
        try:
            fail_stale_inspection_executions()
            dispatch_scheduled_inspections()
        except Exception as exc:
            logger.warning('inspection scheduler loop failed: %s', exc)
        finally:
            close_old_connections()
        threading.Event().wait(SCAN_INTERVAL_SECONDS)


def start_inspection_scheduler_in_background():
    """随 runserver 启动，模式与 gRPC Server 一致（见 grpc_transfer/server.py）。"""
    global _scheduler_started
    argv = sys.argv
    if len(argv) < 2 or argv[1] != 'runserver':
        return
    # autoreload 下只在真正处理请求的子进程启动，避免父子进程重复分发同一个任务。
    if '--noreload' not in argv and os.environ.get('RUN_MAIN') != 'true':
        return
    with _scheduler_lock:
        if _scheduler_started:
            return
        _scheduler_started = True
    threading.Thread(target=_scheduler_loop, daemon=True, name='inspection-scheduler').start()
    logger.info('inspection scheduler started')
