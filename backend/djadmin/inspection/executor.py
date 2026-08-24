from concurrent.futures import ThreadPoolExecutor, as_completed
import socket
import urllib.error
import urllib.request
import uuid

from django.db import close_old_connections
from django.utils import timezone

from assets.application_variables import resolve_application_variables
from assets.grpc_transfer.client import AgentChannelClient

from .models import InspectionExecution, InspectionGroup, InspectionResult, InspectionTargetExecution


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
        compiled.append({
            'key': f'inspection:{execution.pk}:{index}',
            'type': 'shell',
            'name': check.get('name') or f'检查项 {index + 1}',
            'executor': 'shell',
            'command': resolve(config.get('command', '')),
            'run_user': resolve(config.get('run_user') or default_run_user),
            'work_directory': resolve(config.get('work_directory') or default_work_directory),
            'expected': resolve(config.get('expected_output', '')),
            'requires_running': False,
            'environment': environment,
        })
    return {
        'control_type': 'command',
        'check_plan': {'schema_version': 1, 'required_capabilities': ['shell:v1'], 'checks': compiled},
    }


def _persist_results(target, checks):
    rows = []
    for index, check in enumerate(checks if isinstance(checks, list) else []):
        if not isinstance(check, dict):
            continue
        check_key = str(check.get('key') or '')
        if not check_key.startswith('inspection:'):
            continue
        rows.append(InspectionResult(
            target=target,
            check_key=check_key,
            check_type=str(check.get('type') or ''),
            name=str(check.get('name') or f'检查项 {index + 1}'),
            status=str(check.get('status') or 'error'),
            expected_value=check.get('expected'),
            actual_value=check.get('actual'),
            message=str(check.get('message') or ''),
        ))
    InspectionResult.objects.bulk_create(rows)


def _run_agent_target(execution_id, target_id):
    close_old_connections()
    target = InspectionTargetExecution.objects.select_related(
        'deployment__host', 'deployment__deployment_template', 'deployment__application_version', 'host',
    ).get(pk=target_id)
    deployment = target.deployment
    host = target.host
    target.status = InspectionTargetExecution.Status.RUNNING
    target.start_time = timezone.now()
    target.save(update_fields=['status', 'start_time', 'update_time'])
    try:
        if deployment is None and host is None:
            raise RuntimeError('Agent 巡检目标不存在')
        execution = InspectionExecution.objects.get(pk=execution_id)
        result = AgentChannelClient(target.agent_id_snapshot, timeout=execution.task_snapshot['timeout_seconds']).execute_automation(
            job_id=f'inspection-{execution_id}-{target_id}-{uuid.uuid4().hex[:8]}',
            params=_agent_params(execution, deployment=deployment, host=host),
            timeout_seconds=int(execution.task_snapshot['timeout_seconds']),
            task_type='custom',
            action='check_application_baseline',
        )
        raw_data = result.get('result_data')
        data = raw_data if isinstance(raw_data, dict) else {}
        passed = bool(data.get('passed'))
        target.status = InspectionTargetExecution.Status.SUCCESS if passed else InspectionTargetExecution.Status.FAILED
        target.passed = passed
        target.raw_result = data
        target.error_message = str(result.get('error_message') or '')
        _persist_results(target, data.get('checks'))
    except Exception as exc:
        target.status = InspectionTargetExecution.Status.FAILED
        target.passed = False
        target.error_message = str(exc)
    target.end_time = timezone.now()
    target.save(update_fields=['status', 'passed', 'raw_result', 'error_message', 'end_time', 'update_time'])
    close_old_connections()


def _controller_check(check, timeout):
    config = check.get('config') if isinstance(check.get('config'), dict) else {}
    executor = check.get('executor')
    if executor == 'http':
        url = str(config.get('url') or '').strip()
        expected_status = int(config.get('expected_status') or 200)
        try:
            with urllib.request.urlopen(url, timeout=timeout) as response:
                actual = response.status
            passed = actual == expected_status
            return passed, expected_status, actual, '' if passed else f'HTTP 状态码为 {actual}'
        except (urllib.error.URLError, ValueError) as exc:
            return False, expected_status, None, str(exc)
    host = str(config.get('host') or '').strip()
    port = int(config.get('port') or 0)
    try:
        with socket.create_connection((host, port), timeout=timeout):
            pass
        return True, 'connected', 'connected', ''
    except (OSError, ValueError) as exc:
        return False, 'connected', 'failed', str(exc)


def _run_controller_target(execution, target):
    target.status = InspectionTargetExecution.Status.RUNNING
    target.start_time = timezone.now()
    target.save(update_fields=['status', 'start_time', 'update_time'])
    all_passed = True
    for index, check in enumerate(execution.group_snapshot.get('checks', [])):
        passed, expected, actual, message = _controller_check(check, execution.task_snapshot['timeout_seconds'])
        all_passed = all_passed and passed
        InspectionResult.objects.create(
            target=target,
            check_key=f'inspection:{execution.pk}:{index}',
            check_type=str(check.get('executor') or ''),
            name=str(check.get('name') or f'检查项 {index + 1}'),
            status='pass' if passed else 'fail',
            expected_value=expected,
            actual_value=actual,
            message=message,
        )
    target.status = InspectionTargetExecution.Status.SUCCESS if all_passed else InspectionTargetExecution.Status.FAILED
    target.passed = all_passed
    target.end_time = timezone.now()
    target.save(update_fields=['status', 'passed', 'end_time', 'update_time'])


def run_inspection_execution(execution_id):
    close_old_connections()
    execution = InspectionExecution.objects.select_related('task').get(pk=execution_id)
    execution.status = InspectionExecution.Status.RUNNING
    execution.start_time = timezone.now()
    execution.save(update_fields=['status', 'start_time', 'update_time'])
    targets = list(InspectionTargetExecution.objects.filter(execution=execution))
    if execution.group_snapshot.get('scope') == InspectionGroup.Scope.PER_DEPLOYMENT:
        concurrency = max(1, min(int(execution.task_snapshot.get('concurrency') or 20), 100))
        with ThreadPoolExecutor(max_workers=concurrency, thread_name_prefix='inspection') as pool:
            futures = [pool.submit(_run_agent_target, execution.pk, target.pk) for target in targets]
            for future in as_completed(futures):
                future.result()
    elif targets:
        _run_controller_target(execution, targets[0])
    statuses = list(InspectionTargetExecution.objects.filter(execution=execution).values_list('status', flat=True))
    success_count = statuses.count(InspectionTargetExecution.Status.SUCCESS)
    failed_count = statuses.count(InspectionTargetExecution.Status.FAILED)
    execution.status = InspectionExecution.Status.SUCCESS if statuses and failed_count == 0 else InspectionExecution.Status.FAILED
    execution.summary = {'total': len(statuses), 'success': success_count, 'failed': failed_count}
    execution.end_time = timezone.now()
    execution.save(update_fields=['status', 'summary', 'end_time', 'update_time'])
    close_old_connections()