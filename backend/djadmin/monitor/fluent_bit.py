"""Fluent Bit 配置生成（《日志采集架构文档》第 8 节）。

纯渲染逻辑：给定主机上启用的服务实例，生成 inputs.d / outputs.d 片段与
配置指纹。写入主机与热重载由 dj-agent 链路执行（阶段 5），本模块不触碰网络。
"""
import hashlib
import re

from assets.application_variables import resolve_application_variables, resolve_macro_variables
from assets.models import ApplicationDeployment

from .log_management import build_index_name

# 主机侧目录结构（§8.1），offset 数据库必须持久化在 /var/lib 下。
FLUENT_BIT_HOME = '/etc/fluent-bit'
FLUENT_BIT_INPUTS_DIR = f'{FLUENT_BIT_HOME}/inputs.d'
FLUENT_BIT_OUTPUTS_DIR = f'{FLUENT_BIT_HOME}/outputs.d'
FLUENT_BIT_STATE_DIR = '/var/lib/fluent-bit'
FLUENT_BIT_BOOTSTRAP_FRAGMENT = '_djadmin_bootstrap.conf'
FLUENT_BIT_RELOAD_URL = 'http://127.0.0.1:2020/api/v2/reload'

_TAG_SAFE_RE = re.compile(r'[^A-Za-z0-9_.-]+')


def _safe_tag_part(value):
    return _TAG_SAFE_RE.sub('-', str(value or '').strip()) or 'unknown'


def render_main_config():
    """主配置，安装时一次写入；开启热重载与本地状态接口（§8.1/§8.5）。"""
    return (
        '[SERVICE]\n'
        '    Flush                  5\n'
        '    Log_Level              info\n'
        '    Hot_Reload             On\n'
        '    HTTP_Server            On\n'
        '    HTTP_Listen            127.0.0.1\n'
        '    HTTP_Port              2020\n'
        f'    storage.path           {FLUENT_BIT_STATE_DIR}/storage/\n'
        '\n'
        '@INCLUDE inputs.d/*.conf\n'
        '@INCLUDE outputs.d/*.conf\n'
    )


def _fluent_regex(value):
    """转义 Fluent Bit Rule 的引号与斜杠分隔符，不改变正则自身语义。"""
    return str(value).replace('"', r'\"').replace('/', r'\/')


def render_input_fragment(
    *,
    application_code,
    service_code,
    instance_name,
    log_name,
    log_path,
    tag,
    multiline_rule=None,
    input_format='text',
    encoding='utf-8',
    records,
):
    """单个实例单条日志的 inputs.d 片段（§8.2）。

    Tag 按应用、逻辑服务、实例、日志名四段隔离，确保每条日志只命中自己的 output。
    """
    parser_name = ''
    lines = []
    if multiline_rule:
        parser_name = f'multiline_{_safe_tag_part(tag)}'
        lines += [
            '[MULTILINE_PARSER]',
            f'    Name          {parser_name}',
            '    Type          regex',
            f'    Flush_Timeout {multiline_rule.flush_timeout}',
            f'    Rule          "start_state" "/{_fluent_regex(multiline_rule.start_pattern)}/" "continuation"',
            f'    Rule          "continuation" "/{_fluent_regex(multiline_rule.continuation_pattern)}/" "continuation"',
            '',
        ]
    lines += [
        '[INPUT]',
        '    Name              tail',
        f'    Path              {log_path}',
        f'    Tag               {tag}',
        # DB 记录 offset，热重载后从断点续采，不丢日志（§8.5）。
        f'    DB                {FLUENT_BIT_STATE_DIR}/{_safe_tag_part(application_code)}'
        f'__{_safe_tag_part(service_code)}__{_safe_tag_part(instance_name)}__{_safe_tag_part(log_name)}.db',
    ]
    if parser_name:
        # 多行合并必须在采集侧完成，堆栈跨行到后端已无法还原（§5.1）。
        lines.append(f'    Multiline.parser  {parser_name}')
    if input_format == 'json':
        lines.append('    Parser            json')
    lines += [
        f'    Encoding          {encoding or "utf-8"}',
        '    Refresh_Interval  10',
        '    Skip_Long_Lines   On',
        '',
        '[FILTER]',
        '    Name    record_modifier',
        f'    Match   {tag}',
    ]
    for key, value in records.items():
        lines.append(f'    Record  {key} {value}')
    return '\n'.join(lines) + '\n'


def render_output_fragment(*, application_code, service_code, log_name, index, pipeline):
    """按应用类型分组的 outputs.d 片段（§8.3）。

    凭据经 systemd Environment= 注入（OS_HOST/OS_PORT/OS_USER/OS_PASSWORD），不写入配置文件。
    """
    lines = [
        '[OUTPUT]',
        '    Name                opensearch',
        f'    Match               {_safe_tag_part(application_code)}.{_safe_tag_part(service_code)}.*.{_safe_tag_part(log_name)}',
        '    Host                ${OS_HOST}',
        '    Port                ${OS_PORT}',
        '    HTTP_User           ${OS_USER}',
        '    HTTP_Passwd         ${OS_PASSWORD}',
        '    tls                 On',
        f'    Index               {index}',
    ]
    if pipeline:
        lines.append(f'    Pipeline            {pipeline}')
    lines += [
        '    Suppress_Type_Name  On',
        '    Retry_Limit         5',
    ]
    return '\n'.join(lines) + '\n'


def input_fragment_filename(application_code, service_code, instance_name, log_name):
    """每条日志独立片段，避免同一实例的多个日志定义互相覆盖。"""
    return (
        f'{_safe_tag_part(application_code)}__{_safe_tag_part(service_code)}'
        f'__{_safe_tag_part(instance_name)}'
        f'__{_safe_tag_part(log_name)}.conf'
    )


def output_fragment_filename(application_code, service_code, log_name):
    return (
        f'{_safe_tag_part(application_code)}__{_safe_tag_part(service_code)}'
        f'__{_safe_tag_part(log_name)}.conf'
    )


def config_fingerprint(fragment_contents):
    """对全部下发内容计算 sha256，与 LogCollectionTarget.config_fingerprint 比对实现幂等下发（§8.4）。"""
    digest = hashlib.sha256()
    for content in sorted(fragment_contents):
        digest.update(content.encode('utf-8'))
        digest.update(b'\x00')
    return digest.hexdigest()


def resolve_log_path(log_definition, service, deployment):
    """展开 ${APP_HOME}/${RUN_USER}/宏，得到主机上的绝对日志路径（§8.2）。"""
    template = service.deployment_template
    resolved = resolve_application_variables(
        log_definition.path_pattern,
        app_home=template.app_home,
        run_user=template.run_user,
    )
    return resolve_macro_variables(
        resolved,
        definitions=template.macro_definitions or [],
        values=service.macro_values or {},
    )


def build_host_fragments(host, cluster):
    """汇总主机上所有「服务开关 ON 且日志定义 ON」的实例，生成片段清单。

    返回 {'inputs': {filename: content}, 'outputs': {filename: content}, 'fingerprint': str}。
    同主机内展开后的日志路径必须唯一，否则 Fluent Bit 产生 harvester 冲突（§12）。
    """
    inputs = {}
    outputs = {}
    seen_paths = {}

    deployments = (
        ApplicationDeployment.objects
        .filter(host=host, enabled=True)
        .select_related('host')
        .prefetch_related('application_services__deployment_template__logs',
                          'application_services__application',
                          'application_services__application_version',
                          'application_services__business_system',
                          'application_services__environment')
        .order_by('id')
    )

    for deployment in deployments:
        service = deployment.service
        if service is None or not service.log_collection_enabled or not service.enabled:
            continue
        template = service.deployment_template
        application_code = service.application.code
        index = build_index_name(
            cluster.index_prefix,
            service.environment.code if service.environment else 'unknown',
            service.business_system.code,
            service.log_retention_tier,
        )

        for log_def in template.logs.all():
            if not log_def.collection_enabled:
                continue
            log_path = resolve_log_path(log_def, service, deployment)
            # 同主机路径冲突校验（§12）：不同实例展开出相同路径必须拦截。
            previous = seen_paths.get(log_path)
            if previous is not None:
                raise ValueError(
                    f'日志路径冲突: {log_path} 同时被 {previous} 与 '
                    f'{deployment.instance_name}/{log_def.name} 使用'
                )
            seen_paths[log_path] = f'{deployment.instance_name}/{log_def.name}'

            rule = log_def.processing_rule if log_def.processing_rule_id else None
            tag = (
                f'{_safe_tag_part(application_code)}.{_safe_tag_part(service.code)}.'
                f'{_safe_tag_part(deployment.instance_name)}.'
                f'{_safe_tag_part(log_def.name)}'
            )
            pipeline = rule.name if rule else ''
            records = {
                'business_system': service.business_system.code,
                'environment': service.environment.code if service.environment else 'unknown',
                'service': service.code,
                'instance': deployment.instance_name,
                'application': application_code,
                'version': service.application_version.version,
                'host_ip': host.ip or '',
                'log_name': log_def.name,
                # 附加标签进固定前缀，落到 OpenSearch 的 labels flat_object（§4.7）。
                **{f'labels_{key}': value for key, value in (log_def.extra_fields or {}).items()},
            }
            filename = input_fragment_filename(
                application_code, service.code, deployment.instance_name, log_def.name,
            )
            inputs[filename] = render_input_fragment(
                application_code=application_code,
                service_code=service.code,
                instance_name=deployment.instance_name,
                log_name=log_def.name,
                log_path=log_path,
                tag=tag,
                multiline_rule=rule if rule and rule.multiline_enabled else None,
                input_format=rule.input_format if rule else 'text',
                encoding=log_def.encoding,
                records=records,
            )
            # 同一逻辑服务的同名日志多实例共用 output，不同日志与服务严格隔离。
            out_name = output_fragment_filename(application_code, service.code, log_def.name)
            if out_name not in outputs:
                outputs[out_name] = render_output_fragment(
                    application_code=application_code,
                    service_code=service.code,
                    log_name=log_def.name,
                    index=index,
                    pipeline=pipeline,
                )

    fingerprint = config_fingerprint([*inputs.values(), *outputs.values()])
    return {'inputs': inputs, 'outputs': outputs, 'fingerprint': fingerprint}
