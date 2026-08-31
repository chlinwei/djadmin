"""Fluent Bit 配置下发服务（《日志采集架构文档》§8.4/§8.5/§8.6）。

链路：build_host_fragments 生成片段 → 指纹比对（一致跳过）→ dj-agent gRPC
写入 inputs.d/outputs.d → HTTP POST 热重载（不用 systemctl restart，重载后
从 offset 断点续采）→ 更新 LogCollectionTarget 指纹与状态。

配置下发与热重载同步执行（秒级，走 agent gRPC）；安装/卸载 Playbook 只同步做校验与
建历史，实际执行交给 monitor.job_runner 的进程内线程池。
"""
import os
import uuid
from urllib.parse import urlsplit

from django.utils import timezone

from assets.grpc_transfer.client import AgentChannelClient, AgentGrpcTransferError
from assets.credential_crypto import decrypt_secret

from .fluent_bit import (
    FLUENT_BIT_BOOTSTRAP_FRAGMENT,
    FLUENT_BIT_HOME,
    FLUENT_BIT_INPUTS_DIR,
    FLUENT_BIT_MULTILINE_PARSERS_FILE,
    FLUENT_BIT_OUTPUTS_DIR,
    FLUENT_BIT_PARSERS_DIR,
    FLUENT_BIT_RELOAD_URL,
    build_host_fragments,
    render_main_config,
)
from .models import LogCollectionTarget, OpenSearchCluster

# 下发与重载的超时：文件操作走默认超时即可；热重载给 15s（Fluent Bit 全局重初始化所有 pipeline）。
RELOAD_TIMEOUT_SECONDS = 15


class LogCollectionApplyError(Exception):
    """下发失败时携带可展示给用户的错误信息。"""


def _configure_opensearch_output(client, cluster):
    endpoint = urlsplit(cluster.host_list[0])
    if not endpoint.hostname:
        raise LogCollectionApplyError('OpenSearch 连接地址无效')
    port = endpoint.port or (443 if endpoint.scheme == 'https' else 80)
    result = client.execute_automation(
        job_id='fluent-bit-opensearch-config',
        params={
            'host': endpoint.hostname,
            'port': str(port),
            'username': cluster.username,
            'password': decrypt_secret(cluster.password),
        },
        timeout_seconds=30,
        task_type='custom',
        action='configure_fluent_bit_opensearch',
    )
    if result.get('status') != 'success' or result.get('exit_code') not in (0, None):
        raise LogCollectionApplyError(
            f"OpenSearch 输出配置失败: {result.get('error_message') or result.get('stderr') or result.get('status')}"
        )


def _run_fluent_bit_job(action, target_id, history_id, delete_after_success=False):
    """线程池入口：重新加载 ORM 对象，异常统一写回目标状态，避免停在 pending。"""
    from .models import MonitorTargetInstallHistory

    target = LogCollectionTarget.objects.select_related('host').get(id=target_id)
    history = MonitorTargetInstallHistory.objects.get(id=history_id)
    try:
        if action == 'install':
            execute_fluent_bit_install(target, history)
        else:
            execute_fluent_bit_uninstall(target, history, delete_after_success=delete_after_success)
    except Exception as exc:
        LogCollectionTarget.objects.filter(id=target_id).update(
            install_status=LogCollectionTarget.InstallStatus.FAILED,
            install_message=f'Fluent Bit 任务执行异常：{exc}',
        )
        raise


def _select_fluent_bit_package(target):
    from assets.host_info import refresh_host_info
    from assets.models import Host
    from .models import SoftwarePackage
    from .package_selector import PackageSelectionError, normalize_host_platform, select_software_package

    host = target.host
    platform = normalize_host_platform(host)
    if not all(platform.values()):
        refresh_result = refresh_host_info(host)
        if not refresh_result.get('updated'):
            reason = str(refresh_result.get('error') or '主机信息采集失败').strip()
            raise LogCollectionApplyError(
                f'无法识别 Fluent Bit 目标平台：{reason}；请确认 dj-agent 在线后重新采集主机信息'
            )
        # 刷新会更新 HostSystem/HostHardware，重新查询以避开 OneToOne 关系缓存中的旧值。
        host = Host.objects.select_related('system', 'hardware').get(pk=host.pk)
        platform = normalize_host_platform(host)
        if not all(platform.values()):
            raise LogCollectionApplyError(
                'dj-agent 未上报完整的平台信息（ID/VERSION_ID/架构）；'
                '请升级并重启 dj-agent 后重新采集主机信息'
            )

    try:
        return select_software_package(
            host, 'fluent-bit', SoftwarePackage.PackageType.FLUENT_BIT,
        )
    except PackageSelectionError as exc:
        raise LogCollectionApplyError(str(exc)) from exc


def dispatch_fluent_bit_install(target, manual=True):
    """校验安装前置条件、建历史并入队；Playbook 执行在后台线程池里跑。

    校验同步做（错误立刻回给调用方，批量场景能逐台报原因），执行必须异步：
    单台安装可能数分钟，留在请求线程会被 ASGI 超时杀掉。
    """
    from .job_runner import submit_monitor_job
    from .models import MonitorTargetInstallHistory

    active_history = MonitorTargetInstallHistory.objects.filter(
        log_collection_target=target,
        status__in=[
            MonitorTargetInstallHistory.Status.PENDING,
            MonitorTargetInstallHistory.Status.RUNNING,
        ],
    ).order_by('-id').first()
    if active_history is not None:
        raise LogCollectionApplyError('Fluent Bit 安装/卸载任务正在执行，请等待完成或先取消任务')

    package = _select_fluent_bit_package(target)
    if package.install_playbook_template is None:
        raise LogCollectionApplyError('匹配的 Fluent Bit 软件包未配置安装 Playbook')
    file_field = getattr(package, 'file', None)
    if not file_field or not getattr(file_field, 'name', ''):
        raise LogCollectionApplyError('匹配的 Fluent Bit 软件包没有可用离线文件')
    try:
        package_local_path = file_field.path
    except (NotImplementedError, ValueError) as exc:
        raise LogCollectionApplyError('Fluent Bit 软件包存储不支持本地文件传输') from exc
    if not os.path.isfile(package_local_path):
        raise LogCollectionApplyError('Fluent Bit 离线包文件不存在，请重新上传')

    history = MonitorTargetInstallHistory.objects.create(
        log_collection_target=target,
        host=target.host,
        action=MonitorTargetInstallHistory.Action.INSTALL,
        trigger_type=(
            MonitorTargetInstallHistory.TriggerType.MANUAL
            if manual else MonitorTargetInstallHistory.TriggerType.AUTO
        ),
        status=MonitorTargetInstallHistory.Status.PENDING,
        host_id_snapshot=int(getattr(target.host, 'id', 0) or 0),
        host_name_snapshot=str(getattr(target.host, 'instance_name', '') or ''),
        host_ip_snapshot=str(getattr(target.host, 'ip', '') or ''),
        exporter_type_snapshot='fluent-bit',
        summary_message='已下发 Fluent Bit 安装任务',
        start_time=timezone.now(),
    )
    target.install_status = LogCollectionTarget.InstallStatus.PENDING
    target.install_message = '已下发 Fluent Bit 安装任务'
    target.last_dispatch_manual = bool(manual)
    target.save(update_fields=['install_status', 'install_message', 'last_dispatch_manual', 'update_time'])

    submit_monitor_job(_run_fluent_bit_job, 'install', int(target.pk), int(history.pk))
    return history


def execute_fluent_bit_install(target, history):
    """后台线程内执行安装 Playbook 并回写目标状态。"""
    from .models import MonitorTargetInstallHistory
    from .playbook_runner import _resolve_monitor_timeout_seconds, run_monitor_playbook_and_update_history

    package = _select_fluent_bit_package(target)
    template = package.install_playbook_template
    if template is None:
        raise LogCollectionApplyError('匹配的 Fluent Bit 软件包未配置安装 Playbook')
    package_local_path = package.file.path

    run_monitor_playbook_and_update_history(
        target=target,
        host=target.host,
        history=history,
        template_content=template.content or '',
        extra_vars={
            'package_local_path': package_local_path,
            'package_local_directory': os.path.dirname(package_local_path),
            'package_file_name': os.path.basename(package_local_path),
            'package_format': package.package_format,
            'package_platform_family': package.platform_family,
            'package_platform_major': package.platform_major,
            'package_sha256': package.sha256,
            'service_name': package.service_unit_name,
            'fluent_bit_main_config': render_main_config(),
        },
        work_directory=package.work_directory or '/tmp',
        timeout_seconds=_resolve_monitor_timeout_seconds(),
    )
    target.refresh_from_db()
    history.refresh_from_db()
    if history.status != MonitorTargetInstallHistory.Status.SUCCESS:
        return history
    target.agent_installed = True
    target.agent_version = package.version
    target.save(update_fields=['agent_installed', 'agent_version', 'update_time'])
    try:
        # Playbook 退出码为 0 不代表 Fluent Bit 真的常驻运行；必须实测 systemd 状态，
        # 否则误标 RUNNING 会让后续下发误判“不需要 start”，直接热重载到一个没在监听的进程上。
        refresh_target_status(target)
    except LogCollectionApplyError:
        target.runtime_status = LogCollectionTarget.RuntimeStatus.UNKNOWN
        target.save(update_fields=['runtime_status', 'update_time'])
    return history


def dispatch_fluent_bit_uninstall(target, manual=True, delete_after_success=False):
    """校验卸载前置条件、建历史并入队；Playbook 执行在后台线程池里跑。

    delete_after_success=True 用于删除场景：卸载成功后才删掉纳管记录，
    卸载失败则保留记录和日志，避免主机上残留进程失联。
    """
    from .job_runner import submit_monitor_job
    from .models import MonitorTargetInstallHistory

    package = _select_fluent_bit_package(target)
    if package.uninstall_playbook_template is None:
        raise LogCollectionApplyError('匹配的 Fluent Bit 软件包未配置卸载 Playbook')
    history = MonitorTargetInstallHistory.objects.create(
        log_collection_target=target,
        host=target.host,
        action=MonitorTargetInstallHistory.Action.UNINSTALL,
        trigger_type=(
            MonitorTargetInstallHistory.TriggerType.MANUAL
            if manual else MonitorTargetInstallHistory.TriggerType.AUTO
        ),
        status=MonitorTargetInstallHistory.Status.PENDING,
        host_id_snapshot=int(getattr(target.host, 'id', 0) or 0),
        host_name_snapshot=str(getattr(target.host, 'instance_name', '') or ''),
        host_ip_snapshot=str(getattr(target.host, 'ip', '') or ''),
        exporter_type_snapshot='fluent-bit',
        summary_message='已下发 Fluent Bit 卸载任务',
        start_time=timezone.now(),
    )
    target.install_status = LogCollectionTarget.InstallStatus.PENDING
    target.install_message = '已下发 Fluent Bit 卸载任务'
    target.last_dispatch_manual = bool(manual)
    target.save(update_fields=['install_status', 'install_message', 'last_dispatch_manual', 'update_time'])

    submit_monitor_job(
        _run_fluent_bit_job, 'uninstall', int(target.pk), int(history.pk),
        delete_after_success=bool(delete_after_success),
    )
    return history


def execute_fluent_bit_uninstall(target, history, delete_after_success=False):
    """后台线程内执行卸载 Playbook；卸载成功且要求删除时一并删掉纳管记录。"""
    from .models import MonitorTargetInstallHistory
    from .playbook_runner import _resolve_monitor_timeout_seconds, run_monitor_playbook_and_update_history

    package = _select_fluent_bit_package(target)
    template = package.uninstall_playbook_template
    if template is None:
        raise LogCollectionApplyError('匹配的 Fluent Bit 软件包未配置卸载 Playbook')
    run_monitor_playbook_and_update_history(
        target=target,
        host=target.host,
        history=history,
        template_content=template.content or '',
        extra_vars={
            'package_format': package.package_format,
            'service_name': package.service_unit_name,
        },
        work_directory=package.work_directory or '/tmp',
        timeout_seconds=_resolve_monitor_timeout_seconds(),
    )
    target.refresh_from_db()
    history.refresh_from_db()
    if history.status != MonitorTargetInstallHistory.Status.SUCCESS:
        return history
    if delete_after_success:
        target.delete()
        return history
    target.agent_installed = False
    target.agent_version = ''
    target.runtime_status = LogCollectionTarget.RuntimeStatus.UNKNOWN
    target.config_fingerprint = ''
    target.save(update_fields=[
        'agent_installed', 'agent_version', 'runtime_status', 'config_fingerprint', 'update_time',
    ])
    return history


def control_fluent_bit_service(target, action):
    """通过 Agent 对 fluent-bit.service 执行启动或停止，并同步刷新目标状态。"""
    if action not in {'start', 'stop'}:
        raise LogCollectionApplyError('Fluent Bit 服务动作仅支持 start/stop')
    agent_id = str(getattr(target.host, 'agent_id', '') or '').strip()
    if not agent_id:
        raise LogCollectionApplyError('主机未绑定 agent 实例，无法控制 Fluent Bit 服务')
    agent_action = 'start_exporter' if action == 'start' else 'stop_exporter'
    try:
        client = AgentChannelClient(agent_id)
        result = client.execute_automation(
            job_id=f'fluent-bit-{action}-{uuid.uuid4().hex[:16]}',
            params={'service_name': 'fluent-bit.service'},
            timeout_seconds=30,
            task_type='custom',
            action=agent_action,
        )
    except AgentGrpcTransferError as exc:
        raise LogCollectionApplyError(str(exc)) from exc
    succeeded = result.get('status') == 'success' and result.get('exit_code') in (0, None)
    if succeeded:
        target.runtime_status = (
            LogCollectionTarget.RuntimeStatus.RUNNING
            if action == 'start' else LogCollectionTarget.RuntimeStatus.STOPPED
        )
        target.last_error = ''
    else:
        target.runtime_status = LogCollectionTarget.RuntimeStatus.ERROR
        target.last_error = str(result.get('stderr') or result.get('error_message') or '服务操作失败')[:1000]
    target.save(update_fields=['runtime_status', 'last_error', 'update_time'])
    return result


def _get_default_cluster():
    cluster = OpenSearchCluster.objects.filter(enabled=True).order_by('-is_default', 'id').first()
    if cluster is None:
        raise LogCollectionApplyError('尚未配置启用的日志存储集群')
    return cluster


def _write_fragment(client, dir_path, filename, content):
    session = client.open_write(dir_path, filename)
    try:
        session.write_chunk(content.encode('utf-8'))
    except Exception:
        # 失败必须 abort 清理 agent 侧临时文件，避免残留半个配置文件。
        try:
            session.close(abort=True)
        except AgentGrpcTransferError:
            pass
        raise
    session.close()


def _trigger_hot_reload(client):
    """经 agent 在主机本地 curl Fluent Bit 热重载接口（§8.5，不用 systemctl restart）。"""
    result = client.execute_automation(
        job_id='fluent-bit-reload',
        params={},
        timeout_seconds=RELOAD_TIMEOUT_SECONDS,
        task_type='custom',
        # 使用 Agent 内建 HTTP 客户端访问本机接口，主机无需安装 curl/wget/Python。
        action='reload_fluent_bit',
    )
    if result.get('status') != 'success' or result.get('exit_code') not in (0, None):
        raise LogCollectionApplyError(
            f"热重载失败: {result.get('error_message') or result.get('stderr') or result.get('status')}"
        )


def _collect_live_fragment_names(client, dir_path):
    try:
        resp = client.list_dir(dir_path)
    except AgentGrpcTransferError:
        return set()
    names = set()
    for entry in getattr(resp, 'entries', []):
        name = str(getattr(entry, 'name', '') or '')
        if name.endswith('.conf'):
            names.add(name)
    return names


def apply_host_log_config(target):
    """把该主机当前应有的 Fluent Bit 配置全量下发并热重载（§8.4 完整流程）。

    幂等：指纹与 LogCollectionTarget.config_fingerprint 一致时跳过写文件与重载。
    清理：服务关闭采集/实例移除后，目标片段集合缩小，多余的存量 .conf 会被删除（§8.6）。
    """
    if target.runtime_status == LogCollectionTarget.RuntimeStatus.ERROR:
        # Agent 重连或 Fluent Bit 恢复后，不能让旧 last_error 永久阻断下发；先获取实时状态。
        refresh_target_status(target)
    if not target.agent_installed:
        raise LogCollectionApplyError('Fluent Bit 尚未安装，请先完成离线安装并检查运行状态')
    needs_start = target.runtime_status != LogCollectionTarget.RuntimeStatus.RUNNING

    host = target.host
    agent_id = str(getattr(host, 'agent_id', '') or '').strip()
    if not agent_id:
        raise LogCollectionApplyError('主机未绑定 agent 实例，无法下发采集配置')

    cluster = _get_default_cluster()
    try:
        fragments = build_host_fragments(host, cluster)
    except ValueError as exc:
        raise LogCollectionApplyError(str(exc)) from exc

    try:
        # 客户端初始化也会校验 agent gRPC 通道，必须纳入同一失败回写链路，避免前端收到 500。
        client = AgentChannelClient(agent_id)
        # 凭据只写入 root 可读的环境文件；每次下发先确保存在，连接参数未变时 Agent 不重启 Fluent Bit。
        _configure_opensearch_output(client, cluster)
        if not needs_start and fragments['fingerprint'] == target.config_fingerprint:
            return {'skipped': True, 'fingerprint': fragments['fingerprint']}
        client.mkdir(FLUENT_BIT_HOME, 'parsers.d')
        client.mkdir(FLUENT_BIT_INPUTS_DIR, '.')
        client.mkdir(FLUENT_BIT_OUTPUTS_DIR, '.')
        _write_fragment(client, FLUENT_BIT_PARSERS_DIR, FLUENT_BIT_MULTILINE_PARSERS_FILE, fragments['parsers'][FLUENT_BIT_MULTILINE_PARSERS_FILE])
        _write_fragment(client, FLUENT_BIT_HOME, 'fluent-bit.conf', render_main_config())

        for filename, content in fragments['inputs'].items():
            _write_fragment(client, FLUENT_BIT_INPUTS_DIR, filename, content)
        for filename, content in fragments['outputs'].items():
            _write_fragment(client, FLUENT_BIT_OUTPUTS_DIR, filename, content)

        # 清理：目标集合之外的历史片段必须删除并随重载卸载，否则 Fluent Bit
        # 会持续采集无人管理的文件（§8.6）。
        desired_inputs = set(fragments['inputs'])
        desired_outputs = set(fragments['outputs'])
        protected_fragments = {FLUENT_BIT_BOOTSTRAP_FRAGMENT}
        for name in (
            _collect_live_fragment_names(client, FLUENT_BIT_INPUTS_DIR)
            - desired_inputs
            - protected_fragments
        ):
            client.delete(os.path.join(FLUENT_BIT_INPUTS_DIR, name))
        for name in (
            _collect_live_fragment_names(client, FLUENT_BIT_OUTPUTS_DIR)
            - desired_outputs
            - protected_fragments
        ):
            client.delete(os.path.join(FLUENT_BIT_OUTPUTS_DIR, name))

        if needs_start:
            # 旧配置导致服务退出时，先写入修复配置再启动；HTTP 热重载无法作用于已停止的进程。
            control_fluent_bit_service(target, 'start')
        else:
            try:
                _trigger_hot_reload(client)
            except LogCollectionApplyError:
                # runtime_status=RUNNING 是上次记录的状态，可能已经和主机实际状态脱节
                # （例如安装刚完成时进程还没真正常驻）；热重载连不上就退化成启一次服务，而不是直接报错卡死。
                result = control_fluent_bit_service(target, 'start')
                if result.get('status') != 'success' or result.get('exit_code') not in (0, None):
                    raise LogCollectionApplyError(
                        f"热重载失败且启动服务也失败: {result.get('error_message') or result.get('stderr') or result.get('status')}"
                    )
    except (AgentGrpcTransferError, LogCollectionApplyError) as exc:
        target.runtime_status = LogCollectionTarget.RuntimeStatus.ERROR
        target.last_error = str(exc)[:1000]
        target.save(update_fields=['runtime_status', 'last_error', 'update_time'])
        raise LogCollectionApplyError(str(exc)) from exc

    now = timezone.now()
    target.config_fingerprint = fragments['fingerprint']
    target.last_applied_time = now
    target.runtime_status = LogCollectionTarget.RuntimeStatus.RUNNING
    target.last_error = ''
    target.save(update_fields=[
        'config_fingerprint', 'last_applied_time', 'runtime_status', 'last_error', 'update_time',
    ])
    return {
        'skipped': False,
        'fingerprint': fragments['fingerprint'],
        'inputs': sorted(fragments['inputs']),
        'outputs': sorted(fragments['outputs']),
        'applied_time': now.isoformat(),
    }


def refresh_target_status(target):
    """查询主机上 fluent-bit 的 systemd 状态，刷新 agent_installed / runtime_status。

    复用 agent 内置 check_exporter_status 动作（systemctl status fluent-bit.service），
    与 monitor exporter 的状态检查走同一条 gRPC 链路（阶段 4 复用纳管体系）。
    """
    host = target.host
    agent_id = str(getattr(host, 'agent_id', '') or '').strip()
    if not agent_id:
        raise LogCollectionApplyError('主机未绑定 agent 实例，无法检查采集器状态')

    try:
        client = AgentChannelClient(agent_id)
        result = client.execute_automation(
            job_id='fluent-bit-status',
            params={'service_name': 'fluent-bit.service'},
            timeout_seconds=30,
            task_type='custom',
            action='check_exporter_status',
        )
    except AgentGrpcTransferError as exc:
        # Agent 在线字段是最近一次心跳缓存，真正执行前仍可能已断开 gRPC 通道。
        target.last_error = str(exc)[:1000]
        target.save(update_fields=['last_error', 'update_time'])
        raise LogCollectionApplyError(str(exc)) from exc
    stdout = str(result.get('stdout') or '')
    stderr = str(result.get('stderr') or '')
    combined = f'{stdout}\n{stderr}'
    # systemctl status 在 inactive/failed 时退出码非 0，但 stdout 仍包含状态行，这里按内容判断。
    running = 'active (running)' in combined
    installed = ('fluent-bit.service' in combined) and ('could not be found' not in combined)

    target.agent_installed = installed
    target.runtime_status = (
        LogCollectionTarget.RuntimeStatus.RUNNING
        if running else LogCollectionTarget.RuntimeStatus.STOPPED
    )
    if not installed:
        target.runtime_status = LogCollectionTarget.RuntimeStatus.UNKNOWN
    if target.install_status != LogCollectionTarget.InstallStatus.PENDING:
        target.install_status = (
            LogCollectionTarget.InstallStatus.SUCCESS
            if installed else LogCollectionTarget.InstallStatus.UNKNOWN
        )
        target.install_message = 'Fluent Bit 已安装' if installed else 'Fluent Bit 未安装'
    target.last_error = ''
    target.save(update_fields=[
        'agent_installed', 'runtime_status', 'install_status', 'install_message', 'last_error', 'update_time',
    ])
    return {
        'installed': installed,
        'running': running,
        'stdout': stdout[:2000],
        'stderr': stderr[:1000],
    }


def read_instance_log_tail(target, instance_name, log_name, lines=100):
    """读取该主机上指定实例日志的最近 N 行，供解析调试直接拿真实样例（§5.4 闭环）。"""
    host = target.host
    agent_id = str(getattr(host, 'agent_id', '') or '').strip()
    if not agent_id:
        raise LogCollectionApplyError('主机未绑定 agent 实例')

    # 由 build_host_fragments 复算该实例日志的展开路径，避免前端传路径造成任意文件读取。
    cluster = _get_default_cluster()
    try:
        fragments = build_host_fragments(host, cluster)
    except ValueError as exc:
        raise LogCollectionApplyError(str(exc)) from exc

    expected_filename = None
    from .fluent_bit import input_fragment_filename
    from assets.models import ApplicationDeployment
    deployment = ApplicationDeployment.objects.filter(host=host, instance_name=instance_name).first()
    if deployment is None or deployment.service is None:
        raise LogCollectionApplyError(f'主机上不存在实例 {instance_name}')
    application_code = deployment.service.application.code
    expected_filename = input_fragment_filename(
        application_code, deployment.service.code, instance_name, log_name,
    )

    content = fragments['inputs'].get(expected_filename)
    if content is None:
        raise LogCollectionApplyError(f'实例 {instance_name} 未启用日志采集')

    log_path = None
    for line in content.splitlines():
        stripped = line.strip()
        if stripped.startswith('Path'):
            log_path = stripped.split(None, 1)[1].strip()
            break
    if not log_path:
        raise LogCollectionApplyError('配置中未找到日志路径')

    safe_lines = min(max(int(lines or 100), 1), 1000)
    client = AgentChannelClient(agent_id)
    result = client.execute_automation(
        job_id='fluent-bit-log-tail',
        params={'command': f'tail -n {safe_lines} {log_path}'},
        timeout_seconds=30,
        task_type='custom',
        action='run_shell',
    )
    if result.get('status') != 'success':
        raise LogCollectionApplyError(
            f"读取日志失败: {result.get('error_message') or result.get('stderr') or result.get('status')}"
        )
    return {'log_path': log_path, 'content': result.get('stdout', ''), 'lines': safe_lines}
