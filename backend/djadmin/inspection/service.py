"""巡检执行编排：HTTP 手动触发与定时分发共用同一套目标解析、快照生成与投递逻辑。

拆成独立模块而不是留在 views.py，是为了避免定时链路重复实现一遍目标解析规则，
两处漂移会导致“手动能跑、定时跑出空目标”这类难以复现的问题。
"""
from __future__ import annotations

import threading

from django.db import transaction
from django.utils import timezone

from assets.models import Host

from .executor import run_inspection_execution
from .models import InspectionExecution, InspectionGroup, InspectionTargetExecution


class InspectionRequestError(Exception):
    """巡检无法启动的业务原因（组被禁用、目标为空等）；HTTP 层转 400，定时层写调度日志。"""


def normalize_host_ids(selected_host_ids):
    return [int(item) for item in (selected_host_ids or []) if str(item).lstrip('-').isdigit()]


def resolve_scope_hosts(selected_host_ids):
    """只按已落库的主机 ID 解析，不再展开分组，保证任务范围不会因分组成员变化而静默漂移。"""
    host_ids = normalize_host_ids(selected_host_ids)
    if not host_ids:
        return []
    return list(
        Host.objects.filter(pk__in=host_ids, is_deleted_in_cloud=False).order_by('instance_name', 'id')
    )


def host_scope_name(selected_host_ids):
    """主机范围的展示名，供任务列表「目标」列与执行快照共用，避免两处文案不一致。"""
    host_ids = normalize_host_ids(selected_host_ids)
    return f'{len(host_ids)} 台主机' if host_ids else '未选择范围'


def _offline_fields(host, run_time):
    """Agent 离线的目标不下发作业，直接落一条失败记录，避免一台离线拖垮整批巡检。"""
    if host is not None and host.agent_id and host.agent_online:
        return {}
    return {
        'status': InspectionTargetExecution.Status.FAILED,
        'passed': False,
        'error_message': 'Agent 离线，未执行检查',
        'start_time': run_time,
        'end_time': run_time,
    }


def create_execution(task, *, requested_user_id=None, requested_username='', trigger_type=None):
    """创建巡检执行并在事务提交后于当前进程内启动执行，返回 execution。

    校验失败一律抛 InspectionRequestError，不返回 None，避免调用方漏判。
    """
    trigger_type = trigger_type or InspectionExecution.TriggerType.MANUAL
    if not task.enabled or not task.group.enabled:
        raise InspectionRequestError('巡检任务或巡检组已禁用')

    checks = list(task.group.checks.filter(enabled=True).values('name', 'executor', 'config', 'severity', 'order'))
    if not checks:
        raise InspectionRequestError('巡检组没有启用的检查项')

    service = task.logical_service
    scope = task.group.scope
    deployments = []
    hosts = []
    skipped_no_agent = 0
    if scope == InspectionGroup.Scope.PER_HOST:
        group_hosts = resolve_scope_hosts(task.selected_host_ids)
        if not group_hosts:
            raise InspectionRequestError('巡检任务的主机范围为空')
        # 从未安装 Agent 的主机永远无法巡检，归为跳过；当成失败会虚高 failed 并触发误告警。
        hosts = [item for item in group_hosts if str(item.agent_id or '').strip()]
        skipped_no_agent = len(group_hosts) - len(hosts)
        if not hosts:
            raise InspectionRequestError('主机范围内没有已安装 Agent 的主机')
    else:
        if service is None:
            raise InspectionRequestError('巡检任务未绑定逻辑服务')
        deployments = list(service.deployments.filter(
            enabled=True,
            service_links__service=service,
            service_links__enabled=True,
        ).select_related('host').distinct())
        if not deployments:
            raise InspectionRequestError('逻辑服务没有启用的部署实例')

    run_time = timezone.now()
    deployment_snapshot = [
        {
            'deployment_id': item.pk,
            'instance_name': item.instance_name,
            'host_id': item.host_id,
            'host_ip': str(item.host.ip or ''),
            'agent_id': str(item.host.agent_id or ''),
            'agent_online': item.host.agent_online,
        }
        for item in deployments
    ]
    host_snapshot = [{
        'host_id': item.pk,
        'host_name': str(item.instance_name or f'Host-{item.pk}'),
        'host_ip': str(item.ip or ''),
        'agent_id': str(item.agent_id or ''),
    } for item in hosts]
    target_snapshot = host_snapshot if scope == InspectionGroup.Scope.PER_HOST else deployment_snapshot
    target_context = (
        {
            'target_type': task.target_type,
            'name': host_scope_name(task.selected_host_ids),
            'selected_host_ids': normalize_host_ids(task.selected_host_ids),
            'host_count': len(hosts),
            'skipped_no_agent': skipped_no_agent,
        }
        if scope == InspectionGroup.Scope.PER_HOST else
        {
            'target_type': task.target_type,
            'id': service.pk,
            'name': service.name,
            'code': service.code,
            'topology_type': service.topology_type,
            'cluster_type': service.cluster_profile.cluster_type if service.cluster_profile else '',
            'access_address': service.access_address,
        }
    )

    with transaction.atomic():
        execution = InspectionExecution.objects.create(
            task=task,
            trigger_type=trigger_type,
            task_snapshot={
                'id': task.pk,
                'name': task.name,
                'target_type': task.target_type,
                'concurrency': task.concurrency,
                'timeout_seconds': task.timeout_seconds,
            },
            group_snapshot={'id': task.group_id, 'name': task.group.name, 'scope': task.group.scope, 'checks': checks},
            service_snapshot=target_context,
            target_snapshot=target_snapshot,
            requested_user_id=requested_user_id,
            requested_username=requested_username,
        )
        if scope == InspectionGroup.Scope.PER_HOST:
            InspectionTargetExecution.objects.bulk_create([
                InspectionTargetExecution(
                    execution=execution,
                    host=item,
                    target_name=str(item.instance_name or item.ip or f'Host-{item.pk}'),
                    host_id_snapshot=item.pk,
                    host_ip_snapshot=str(item.ip or ''),
                    agent_id_snapshot=str(item.agent_id or ''),
                    **_offline_fields(item, run_time),
                )
                for item in hosts
            ])
        elif scope == InspectionGroup.Scope.PER_DEPLOYMENT:
            InspectionTargetExecution.objects.bulk_create([
                InspectionTargetExecution(
                    execution=execution,
                    deployment=item,
                    target_name=item.instance_name,
                    host_id_snapshot=item.host_id,
                    host_ip_snapshot=str(item.host.ip or ''),
                    agent_id_snapshot=str(item.host.agent_id or ''),
                    **_offline_fields(item.host, run_time),
                )
                for item in deployments
            ])
        else:
            InspectionTargetExecution.objects.create(execution=execution, target_name=service.name)

        # 必须等事务提交后再启动，否则执行线程可能先于数据可见而查不到 execution。
        # 只能在当前进程内执行：agent 的 gRPC 会话注册表是进程内内存，跨进程看不到任何在线 agent。
        transaction.on_commit(lambda: threading.Thread(
            target=run_inspection_execution,
            args=(execution.pk,),
            daemon=True,
            name=f'inspection-{execution.pk}',
        ).start())
    return execution
