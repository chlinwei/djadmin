from concurrent.futures import ThreadPoolExecutor, wait
from typing import Any

from django.db import close_old_connections
from django.utils import timezone

from assets.application_variables import resolve_application_variables
from assets.grpc_transfer.client import AgentChannelClient

from .alerting import sync_inspection_alert
from .models import (
    InspectionExecution,
    InspectionGroup,
    InspectionResult,
    InspectionSeverity,
    InspectionTargetExecution,
)

PASSING_STATUSES = ('pass', 'skipped')
# 单目标超时之外额外给 gRPC 建连与结果回传留的余量，避免恰好卡在超时边界误判为失败。
TARGET_WAIT_GRACE_SECONDS = 45


def _resolve(value, deployment):
    template = deployment.deployment_template
    text = resolve_application_variables(value, app_home=template.app_home, run_user=template.run_user)
    replacements = {
        '${INSTANCE_NAME}': deployment.instance_name,
        '${APPLICATION_VERSION}': deployment.application_version.version,
        '${HOST_IP}': str(deployment.host.ip or ''),
        '${SERVICE_NAME}': template.service_name,
    }
    for key, replacement in replacements.items():
        text = text.replace(key, str(replacement or ''))
    return text


def _resolve_host(value, host):
    text = str(value or '')
    replacements = {
        '${HOST_IP}': str(host.ip or ''),
        '${HOST_NAME}': str(host.instance_name or f'Host-{host.pk}'),
    }
    for key, replacement in replacements.items():
        text = text.replace(key, replacement)
    return text


def _agent_params(execution, deployment=None, host=None):
    checks = execution.group_snapshot.get('checks', [])
    compiled = []
    required_capabilities = set()
    for index, check in enumerate(checks):
        config = check.get('config') if isinstance(check.get('config'), dict) else {}
        if deployment is not None:
            resolve = lambda value: _resolve(value, deployment)
            default_run_user = '${RUN_USER}'
            default_work_directory = '${APP_HOME}'
            environment = {
                'APP_HOME': _resolve('${APP_HOME}', deployment),
                'RUN_USER': _resolve('${RUN_USER}', deployment),
                'INSTANCE_NAME': deployment.instance_name,
                'APPLICATION_VERSION': deployment.application_version.version,
                'HOST_IP': str(deployment.host.ip or ''),
            }
        elif host is not None:
            resolve = lambda value: _resolve_host(value, host)
            default_run_user = ''
            default_work_directory = '/'
            environment = {'HOST_IP': str(host.ip or ''), 'HOST_NAME': str(host.instance_name or f'Host-{host.pk}')}
        else:
            raise ValueError('Agent 巡检目标缺少部署实例或主机')
        executor = str(check.get('executor') or '')
        compiled_check = {
            'key': f'inspection:{execution.pk}:{index}',
            'type': executor,
            'name': check.get('name') or f'检查项 {index + 1}',
            'executor': executor,
            'requires_running': False,
        }
        if executor == 'schema_validate':
            required_capabilities.add('schema_validate:v1')
            schema_type = str(config.get('schema_type') or '')
            schema_versions = {'json_schema': '2020-12', 'schematron': 'iso', 'regexp': 're2'}
            compiled_check.update({
                'path': resolve(config.get('path', '')),
                'document_type': str(config.get('document_type') or ''),
                'schema': {
                    'type': schema_type,
                    'version': schema_versions.get(schema_type, ''),
                    # Schema 文本是规则本身，不能展开其中可能出现的 ${...} 字面量。
                    'content': str(config.get('schema_content') or ''),
                },
            })
        elif executor == 'shell':
            required_capabilities.add('shell:v1')
            compiled_check.update({
                'command': resolve(config.get('command', '')),
                'run_user': resolve(config.get('run_user') or default_run_user),
                'work_directory': resolve(config.get('work_directory') or default_work_directory),
                'expected': resolve(config.get('expected_output', '')),
                'environment': environment,
            })
        elif executor == 'http':
            required_capabilities.add('http:v1')
            expected_status = int(config.get('expected_status') or 200)
            compiled_check.update({
                'url': resolve(config.get('url', '')),
                'expected_status': expected_status,
                'expected': expected_status,
            })
        elif executor == 'tcp':
            required_capabilities.add('tcp:v1')
            compiled_check.update({
                # 未指定主机时检查执行 Agent 自身，避免调用方猜测管理 IP 或 VIP。
                'host': resolve(config.get('host') or '127.0.0.1'),
                'port': int(config.get('port') or 0),
                'expected': 'connected',
            })
        else:
            raise ValueError(f'不支持的巡检执行器: {executor}')
        compiled.append(compiled_check)
    return {
        'control_type': 'command',
        'check_plan': {
            'schema_version': 1,
            'required_capabilities': sorted(required_capabilities),
            'checks': compiled,
        },
    }


def _severity_by_index(execution):
    """检查项下标 -> 严重级别；check_key 里的下标是结果与巡检组快照的唯一关联方式。"""
    checks = execution.group_snapshot.get('checks', [])
    return {
        index: str(check.get('severity') or InspectionSeverity.CRITICAL)
        for index, check in enumerate(checks if isinstance(checks, list) else [])
    }


def _result_severity(check_key, severity_map):
    # 计划级错误（check_plan）没有下标，属于整批不可执行，一律按严重处理。
    suffix = str(check_key).rsplit(':', 1)[-1]
    if not suffix.isdigit():
        return InspectionSeverity.CRITICAL
    return severity_map.get(int(suffix), InspectionSeverity.CRITICAL)


def _persist_results(target, checks, severity_map):
    rows = []
    for index, check in enumerate(checks if isinstance(checks, list) else []):
        if not isinstance(check, dict):
            continue
        check_key = str(check.get('key') or '')
        # Agent 能力不匹配等计划级错误没有 inspection: 前缀，但它正是目标失败的根因。
        if not check_key.startswith('inspection:') and check_key != 'check_plan':
            continue
        rows.append(InspectionResult(
            target=target,
            check_key=check_key,
            check_type=str(check.get('type') or ''),
            name=str(check.get('name') or f'检查项 {index + 1}'),
            status=str(check.get('status') or 'error'),
            severity=_result_severity(check_key, severity_map),
            expected_value=check.get('expected'),
            actual_value=check.get('actual'),
            message=str(check.get('message') or ''),
        ))
    # 重试或重放同一目标时先清空旧结果，否则同一次执行会出现重复检查项。
    InspectionResult.objects.filter(target=target).delete()
    InspectionResult.objects.bulk_create(rows)
    return rows


def _run_agent_target(execution_id, target_id):
    close_old_connections()
    target = InspectionTargetExecution.objects.select_related(
        'deployment__host', 'host',
    ).get(pk=target_id)
    deployment = target.deployment
    host = target.host
    # 只取执行所需字段：target_snapshot 在大批量目标时很大，每个线程都拉一遍得不偿失。
    execution = InspectionExecution.objects.only('status', 'group_snapshot', 'task_snapshot').get(pk=execution_id)
    if execution.status == InspectionExecution.Status.CANCELED:
        target.status = InspectionTargetExecution.Status.CANCELED
        target.end_time = timezone.now()
        target.save(update_fields=['status', 'end_time', 'update_time'])
        close_old_connections()
        return
    target.status = InspectionTargetExecution.Status.RUNNING
    target.start_time = timezone.now()
    target.save(update_fields=['status', 'start_time', 'update_time'])
    try:
        if deployment is None and host is None:
            raise RuntimeError('Agent 巡检目标不存在')
        result = AgentChannelClient(target.agent_id_snapshot, timeout=execution.task_snapshot['timeout_seconds']).execute_automation(
            job_id=f'inspection-{execution_id}-{target_id}',
            params=_agent_params(execution, deployment=deployment, host=host),
            timeout_seconds=int(execution.task_snapshot['timeout_seconds']),
            task_type='custom',
            action='check_application_baseline',
        )
        execution.refresh_from_db(fields=['status'])
        if execution.status == InspectionExecution.Status.CANCELED:
            target.status = InspectionTargetExecution.Status.CANCELED
            target.end_time = timezone.now()
            target.save(update_fields=['status', 'end_time', 'update_time'])
            close_old_connections()
            return
        raw_data = result.get('result_data')
        data = raw_data if isinstance(raw_data, dict) else {}
        passed = bool(data.get('passed'))
        severity_map = _severity_by_index(execution)
        rows = _persist_results(target, data.get('checks'), severity_map)
        # warning 级检查项失败只计数不判负，避免“磁盘偏高”把整台机器标成巡检失败。
        critical_failed = any(
            row.status not in PASSING_STATUSES and row.severity == InspectionSeverity.CRITICAL
            for row in rows
        )
        if not rows and not passed:
            critical_failed = True
        target.status = InspectionTargetExecution.Status.FAILED if critical_failed else InspectionTargetExecution.Status.SUCCESS
        target.passed = not critical_failed
        target.raw_result = data
        plan_error = next((
            str(check.get('message') or '')
            for check in data.get('checks', [])
            if isinstance(check, dict) and check.get('key') == 'check_plan' and check.get('status') == 'error'
        ), '')
        target.error_message = str(result.get('error_message') or plan_error)
    except Exception as exc:
        execution.refresh_from_db(fields=['status'])
        target.status = (
            InspectionTargetExecution.Status.CANCELED
            if execution.status == InspectionExecution.Status.CANCELED
            else InspectionTargetExecution.Status.FAILED
        )
        target.passed = False
        target.error_message = str(exc)
    target.end_time = timezone.now()
    target.save(update_fields=['status', 'passed', 'raw_result', 'error_message', 'end_time', 'update_time'])
    close_old_connections()


def _select_service_agent(execution, target):
    from assets.models import ApplicationDeployment

    candidate_ids = [item.get('deployment_id') for item in execution.target_snapshot if item.get('deployment_id')]
    candidates = list(ApplicationDeployment.objects.filter(pk__in=candidate_ids).select_related('host').order_by('pk'))
    if not candidates:
        raise RuntimeError('逻辑服务没有可用的部署实例')

    cluster_type = str(execution.service_snapshot.get('cluster_type') or '')
    if cluster_type == 'ha':
        vip = str(execution.service_snapshot.get('access_address') or '').strip()
        if not vip:
            raise RuntimeError('HA 逻辑服务未配置 VIP')
        owners = []
        discovery_errors = []
        for deployment in candidates:
            agent_id = str(deployment.host.agent_id or '')
            if not agent_id:
                discovery_errors.append(f'{deployment.instance_name}: 未配置 Agent')
                continue
            try:
                result = AgentChannelClient(agent_id, timeout=5).execute_automation(
                    job_id=f'inspection-discovery-{execution.pk}-{deployment.pk}',
                    params={'inspection_discovery': 'local_addresses'},
                    timeout_seconds=5,
                    task_type='custom',
                    action='get_local_addresses',
                )
                raw_data = result.get('result_data')
                data = raw_data if isinstance(raw_data, dict) else {}
                if vip in {str(address) for address in data.get('local_ipv4', [])}:
                    owners.append(deployment)
            except Exception as exc:
                discovery_errors.append(f'{deployment.instance_name}: {exc}')
        if len(owners) > 1:
            owner_names = '、'.join(item.instance_name for item in owners)
            raise RuntimeError(f'VIP {vip} 同时存在于多个成员节点: {owner_names}')
        if not owners:
            detail = f'；发现失败: {"；".join(discovery_errors)}' if discovery_errors else ''
            raise RuntimeError(f'未找到持有 VIP {vip} 的在线 Agent{detail}')
        selected = owners[0]
    else:
        selected = next(
            (item for item in candidates if item.host.agent_id and item.host.agent_online),
            None,
        )
        if selected is None:
            raise RuntimeError('逻辑服务没有在线 Agent 可执行巡检')

    target.deployment = selected
    target.target_name = f'{execution.service_snapshot.get("name") or target.target_name} ({selected.instance_name})'
    target.host_id_snapshot = selected.host.pk
    target.host_ip_snapshot = str(selected.host.ip or '')
    target.agent_id_snapshot = str(selected.host.agent_id or '')
    target.save(update_fields=[
        'deployment', 'target_name', 'host_id_snapshot', 'host_ip_snapshot', 'agent_id_snapshot', 'update_time',
    ])


def _summarize(execution, error=''):
    """一律按目标实际状态聚合，避免超时等分支写入与明细对不上的计数。"""
    statuses = list(
        InspectionTargetExecution.objects.filter(execution=execution).values_list('status', flat=True)
    )
    summary: dict[str, Any] = {
        'total': len(statuses),
        'success': statuses.count(InspectionTargetExecution.Status.SUCCESS),
        'failed': statuses.count(InspectionTargetExecution.Status.FAILED),
        'canceled': statuses.count(InspectionTargetExecution.Status.CANCELED),
        'warning': InspectionResult.objects.filter(
            target__execution=execution,
            severity=InspectionSeverity.WARNING,
        ).exclude(status__in=PASSING_STATUSES).count(),
    }
    if error:
        summary['error'] = error
    return summary


def run_inspection_execution(execution_id):
    close_old_connections()
    execution = InspectionExecution.objects.select_related('task').get(pk=execution_id)
    if execution.status == InspectionExecution.Status.CANCELED:
        close_old_connections()
        return
    execution.status = InspectionExecution.Status.RUNNING
    execution.start_time = timezone.now()
    execution.save(update_fields=['status', 'start_time', 'update_time'])
    try:
        targets = list(InspectionTargetExecution.objects.filter(execution=execution))
        if execution.group_snapshot.get('scope') == InspectionGroup.Scope.PER_DEPLOYMENT:
            # 入口已把 Agent 离线的目标直接置为 FAILED，这里只调度仍待执行的，避免把离线原因覆盖掉。
            dispatchable = [item for item in targets if item.status == InspectionTargetExecution.Status.PENDING]
            concurrency = max(1, min(int(execution.task_snapshot.get('concurrency') or 20), 100))
            pool = ThreadPoolExecutor(max_workers=concurrency, thread_name_prefix='inspection')
            futures = [pool.submit(_run_agent_target, execution.pk, target.pk) for target in dispatchable]
            try:
                done, pending = wait(
                    futures,
                    timeout=int(execution.task_snapshot.get('timeout_seconds') or 60) + TARGET_WAIT_GRACE_SECONDS,
                )
                for future in done:
                    future.result()
                if pending:
                    # 先落目标终态再汇总，否则 summary 会把已成功的目标也算成 0。
                    InspectionTargetExecution.objects.filter(
                        execution=execution,
                        status__in=[InspectionTargetExecution.Status.PENDING, InspectionTargetExecution.Status.RUNNING],
                    ).update(status=InspectionTargetExecution.Status.FAILED, passed=False, end_time=timezone.now())
                    execution.refresh_from_db(fields=['status'])
                    if execution.status != InspectionExecution.Status.CANCELED:
                        execution.status = InspectionExecution.Status.FAILED
                        execution.summary = _summarize(execution, '巡检目标超过后端等待时限')
                    return
            finally:
                pool.shutdown(wait=False, cancel_futures=True)
        elif execution.group_snapshot.get('scope') == InspectionGroup.Scope.SERVICE_ONCE and targets:
            target = targets[0]
            try:
                _select_service_agent(execution, target)
                _run_agent_target(execution.pk, target.pk)
            except Exception as exc:
                target.status = InspectionTargetExecution.Status.FAILED
                target.passed = False
                target.error_message = str(exc)
                target.start_time = target.start_time or timezone.now()
                target.end_time = timezone.now()
                target.save(update_fields=['status', 'passed', 'error_message', 'start_time', 'end_time', 'update_time'])
        summary = _summarize(execution)
        execution.refresh_from_db(fields=['status'])
        if execution.status != InspectionExecution.Status.CANCELED:
            execution.status = (
                InspectionExecution.Status.SUCCESS
                if summary['total'] and summary['failed'] == 0
                else InspectionExecution.Status.FAILED
            )
        execution.summary = summary
    except Exception as exc:
        execution.refresh_from_db(fields=['status'])
        if execution.status != InspectionExecution.Status.CANCELED:
            execution.status = InspectionExecution.Status.FAILED
        execution.summary = _summarize(execution, str(exc))
    finally:
        current_status = InspectionExecution.objects.values_list('status', flat=True).get(pk=execution.pk)
        if current_status == InspectionExecution.Status.CANCELED:
            # 取消由接口侧写定，这里只补统计，不回写 status/end_time 覆盖取消现场。
            execution.status = InspectionExecution.Status.CANCELED
            execution.save(update_fields=['summary', 'update_time'])
        else:
            execution.end_time = timezone.now()
            execution.save(update_fields=['status', 'summary', 'end_time', 'update_time'])
        try:
            sync_inspection_alert(execution)
        except Exception as alert_error:
            # 告警投递依赖 broker 与通知配置，它失败不应让已收敛的巡检结果变成任务失败。
            print(f'[INSPECTION] sync alert failed for execution {execution.pk}: {alert_error}')
        close_old_connections()