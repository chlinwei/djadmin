"""日志采集链路对账：逐层比对「期望状态」与「集群/主机实际状态」。

存在意义是消除静默失败——索引模板没下发、pipeline 没发布、主机配置改了没同步，
这些问题原先只能等聚合报 400 或登上主机看 journalctl 才发现。
本模块全程只读，不做任何写入或自动修复。
"""
import json

from django.utils import timezone

from .fluent_bit import build_host_fragments
from .log_management import (
    build_index_template_body,
    build_index_template_name,
    build_ism_policy_body,
    build_ism_policy_name,
)
from .models import LogCollectionTarget, LogProcessingRule, LogRetentionTier
from .opensearch_client import OpenSearchClient, OpenSearchError

STATUS_OK = 'ok'
STATUS_WARN = 'warn'
STATUS_DRIFT = 'drift'
STATUS_ERROR = 'error'

# 数值越大越严重：明细项汇总成层状态、层状态汇总成整体状态都取最严重的一个。
_STATUS_RANK = {STATUS_OK: 0, STATUS_WARN: 1, STATUS_DRIFT: 2, STATUS_ERROR: 3}

DATA_FLOW_WINDOW_MINUTES = 15


def _worst(statuses):
    return max(statuses, key=lambda status: _STATUS_RANK.get(status, 0), default=STATUS_OK)


def _item(name, status, detail=''):
    return {'name': name, 'status': status, 'detail': detail}


def _layer(key, name, status, summary, items=None):
    return {'key': key, 'name': name, 'status': status, 'summary': summary, 'items': items or []}


def _layer_from_items(key, name, items, empty_summary):
    if not items:
        return _layer(key, name, STATUS_WARN, empty_summary)
    abnormal = [entry for entry in items if entry['status'] != STATUS_OK]
    summary = f'{len(items)} 项全部一致' if not abnormal else f'{len(abnormal)}/{len(items)} 项需要处理'
    return _layer(key, name, _worst([entry['status'] for entry in items]), summary, items)


def _is_not_found(exc):
    """OpenSearchError 消息格式为 '<HTTP 状态码>: <响应体>'，据此区分「对象不存在」与「集群异常」。"""
    return str(exc).startswith('404')


def _host_label(target):
    return str(getattr(target.host, 'ip', '') or '') or f'host-{target.host_id}'


def _check_index_template(client, cluster):
    """模板决定字段类型：dynamic 漏成 true 会把 keyword 建成 text，聚合类功能全部失效。"""
    prefix = cluster.index_prefix or 'logs'
    name = build_index_template_name(prefix)
    desired = build_index_template_body(prefix)['template']['mappings']

    try:
        response = client.get_index_template(name)
    except OpenSearchError as exc:
        if _is_not_found(exc):
            return _layer('index_template', '索引模板', STATUS_DRIFT, f'模板 {name} 不存在，新建索引会走动态映射')
        return _layer('index_template', '索引模板', STATUS_ERROR, f'读取模板失败: {exc}')

    templates = response.get('index_templates') or []
    actual = {}
    if templates:
        actual = ((templates[0].get('index_template') or {}).get('template') or {}).get('mappings') or {}

    desired_dynamic = desired.get('dynamic', True)
    actual_dynamic = actual.get('dynamic', True)
    items = [_item(
        'dynamic',
        STATUS_OK if actual_dynamic == desired_dynamic else STATUS_DRIFT,
        f'期望 {desired_dynamic}，实际 {actual_dynamic}',
    )]

    actual_properties = actual.get('properties') or {}
    for field, spec in (desired.get('properties') or {}).items():
        current = actual_properties.get(field)
        if not current:
            items.append(_item(field, STATUS_DRIFT, f"缺失，期望 {spec.get('type')}"))
        elif current.get('type') != spec.get('type'):
            items.append(_item(field, STATUS_DRIFT, f"类型不符：期望 {spec.get('type')}，实际 {current.get('type')}"))
        else:
            items.append(_item(field, STATUS_OK, str(spec.get('type'))))

    return _layer_from_items('index_template', '索引模板', items, '模板无字段定义')


def _ism_signature(policy):
    """只提取影响保留行为的字段：集群会补 policy_id/last_updated_time 等元数据，全量比对必然误报。"""
    states = {str(state.get('name')): state for state in (policy.get('states') or [])}
    hot = states.get('hot') or {}
    rollover = next(
        (action['rollover'] for action in (hot.get('actions') or []) if isinstance(action, dict) and 'rollover' in action),
        {},
    )
    delete_after = next(
        (
            (transition.get('conditions') or {}).get('min_index_age')
            for transition in (hot.get('transitions') or []) if transition.get('state_name') == 'delete'
        ),
        None,
    )
    templates = policy.get('ism_template') or []
    if isinstance(templates, dict):
        templates = [templates]
    return {
        'rollover_size': rollover.get('min_primary_shard_size'),
        'rollover_age': rollover.get('min_index_age'),
        'delete_after': delete_after,
        'index_patterns': sorted(
            str(pattern)
            for entry in templates
            for pattern in (entry.get('index_patterns') or [])
        ),
    }


def _check_ism_policies(client, cluster):
    """ISM 缺失时索引既不滚动也不清理，磁盘会被慢慢撑满，属于静默故障。"""
    prefix = cluster.index_prefix or 'logs'
    items = []
    for tier in LogRetentionTier.objects.filter(enabled=True):
        name = build_ism_policy_name(prefix, tier.code)
        desired = _ism_signature(build_ism_policy_body(prefix, tier)['policy'])
        try:
            actual = _ism_signature((client.get_ism_policy(name) or {}).get('policy') or {})
        except OpenSearchError as exc:
            if _is_not_found(exc):
                items.append(_item(name, STATUS_DRIFT, '策略不存在，索引不会自动滚动与清理'))
            else:
                items.append(_item(name, STATUS_ERROR, str(exc)[:200]))
            continue
        if actual == desired:
            items.append(_item(name, STATUS_OK, f'保留 {tier.retention_days} 天'))
        else:
            differing = [key for key, value in desired.items() if actual.get(key) != value]
            items.append(_item(name, STATUS_DRIFT, f"与档位配置不一致: {', '.join(differing)}"))

    return _layer_from_items('ism_policies', '保留策略', items, '没有启用中的保留档位')


def _pipeline_signature(body):
    """processors 有序，直接按顺序序列化比对；描述等非行为字段不参与。"""
    return json.dumps(
        {'processors': (body or {}).get('processors') or [], 'on_failure': (body or {}).get('on_failure') or []},
        sort_keys=True,
        ensure_ascii=False,
    )


def _check_pipelines(client, cluster):
    """pipeline 没发布时日志照样写入，只是不被解析，界面上看不出任何异常。"""
    items = []
    for rule in LogProcessingRule.objects.filter(cluster=cluster):
        try:
            remote = client.get_pipeline(rule.name) or {}
        except OpenSearchError as exc:
            if _is_not_found(exc):
                items.append(_item(rule.name, STATUS_DRIFT, '集群上不存在该 pipeline，日志不会被解析'))
            else:
                items.append(_item(rule.name, STATUS_ERROR, str(exc)[:200]))
            continue

        body = remote.get(rule.name) or {}
        if not body:
            items.append(_item(rule.name, STATUS_DRIFT, '集群上不存在该 pipeline，日志不会被解析'))
        elif _pipeline_signature(body) != _pipeline_signature(rule.pipeline_body):
            items.append(_item(rule.name, STATUS_DRIFT, '集群上的 pipeline 与页面配置不一致，需重新发布'))
        else:
            processors = (rule.pipeline_body or {}).get('processors') or []
            items.append(_item(rule.name, STATUS_OK, f'{len(processors)} 个处理器'))

    return _layer_from_items('pipelines', '解析规则', items, '尚未配置解析规则')


def _check_host_configs(cluster):
    """比对「当前应有的配置指纹」与「最近一次下发记录的指纹」，暴露改了配置但没下发的主机。

    注意这里比的是数据库记录，不回读主机文件；主机被人手工改过仍会显示一致。
    """
    items = []
    targets = LogCollectionTarget.objects.filter(managed_enabled=True).select_related('host')
    for target in targets:
        label = _host_label(target)
        if not target.agent_installed:
            items.append(_item(label, STATUS_WARN, 'Fluent Bit 未安装'))
            continue
        try:
            fragments = build_host_fragments(target.host, cluster)
        except ValueError as exc:
            items.append(_item(label, STATUS_ERROR, f'配置渲染失败: {exc}'))
            continue

        if not fragments['inputs']:
            # 主机上没有任何逻辑服务日志要采，本就无需下发，报漂移属于噪音。
            items.append(_item(label, STATUS_OK, '无采集项'))
        elif not target.config_fingerprint:
            items.append(_item(label, STATUS_DRIFT, '从未下发过采集配置'))
        elif fragments['fingerprint'] != target.config_fingerprint:
            items.append(_item(label, STATUS_DRIFT, '配置已变更但未下发'))
        else:
            items.append(_item(label, STATUS_OK, f"{len(fragments['inputs'])} 个采集项"))

    return _layer_from_items('host_configs', '主机配置', items, '没有纳管中的采集目标')


def _check_runtime():
    """运行状态取自数据库缓存，反映最近一次探测结果，不是实时探活。"""
    items = []
    targets = LogCollectionTarget.objects.filter(
        managed_enabled=True, agent_installed=True,
    ).select_related('host')
    for target in targets:
        label = _host_label(target)
        if target.runtime_status == LogCollectionTarget.RuntimeStatus.RUNNING:
            items.append(_item(label, STATUS_OK, '运行中'))
        elif target.runtime_status == LogCollectionTarget.RuntimeStatus.ERROR:
            items.append(_item(label, STATUS_ERROR, (target.last_error or '异常')[:200]))
        elif target.runtime_status == LogCollectionTarget.RuntimeStatus.STOPPED:
            items.append(_item(label, STATUS_DRIFT, '已停止'))
        else:
            items.append(_item(label, STATUS_WARN, '状态未知，需刷新'))

    return _layer_from_items('runtime', '采集进程', items, '没有已安装 Fluent Bit 的主机')


def _check_data_flow(client, cluster):
    """前面几层全绿也可能没数据，这一层是唯一能证明链路真正通了的证据。"""
    prefix = cluster.index_prefix or 'logs'
    body = {
        'size': 0,
        'query': {'range': {'@timestamp': {'gte': f'now-{DATA_FLOW_WINDOW_MINUTES}m'}}},
        'aggs': {'by_service': {'terms': {'field': 'service', 'size': 50}}},
    }
    try:
        result = client.search(f'{prefix}-*', body)
    except OpenSearchError as exc:
        return _layer('data_flow', '数据写入', STATUS_ERROR, f'查询失败: {exc}')

    total = ((result.get('hits') or {}).get('total') or {}).get('value') or 0
    buckets = ((result.get('aggregations') or {}).get('by_service') or {}).get('buckets') or []
    items = [_item(str(bucket['key']), STATUS_OK, f"{bucket['doc_count']} 条") for bucket in buckets]

    if not total:
        return _layer(
            'data_flow', '数据写入', STATUS_WARN,
            f'最近 {DATA_FLOW_WINDOW_MINUTES} 分钟没有新日志写入', items,
        )
    return _layer(
        'data_flow', '数据写入', STATUS_OK,
        f'最近 {DATA_FLOW_WINDOW_MINUTES} 分钟写入 {total} 条，覆盖 {len(buckets)} 个服务', items,
    )


def collect_log_pipeline_health(cluster):
    """按「存储 → 解析 → 采集 → 数据」的顺序逐层对账，任一层异常都会冒泡到整体状态。"""
    client = OpenSearchClient(cluster)
    layers = [
        _check_index_template(client, cluster),
        _check_ism_policies(client, cluster),
        _check_pipelines(client, cluster),
        _check_host_configs(cluster),
        _check_runtime(),
        _check_data_flow(client, cluster),
    ]
    return {
        'status': _worst([layer['status'] for layer in layers]),
        'checked_at': timezone.now().isoformat(),
        'layers': layers,
    }
