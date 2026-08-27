"""日志存储（OpenSearch）侧的管理面构建器。

对应《日志采集架构文档》第 4/5 节：索引命名、index template、ISM policy、
ingest pipeline 的默认体。全部为纯函数，不触碰网络，便于单测；
bootstrap_log_storage 是唯一执行写入的编排入口。
"""
import re

from .opensearch_client import OpenSearchClient

# 保留档位固定 3 个，不随业务扩张；rollover 的 min_index_age 按档位区分（§4.5）。
RETENTION_TIERS = {
    'hot': {'retention_days': 7, 'rollover_min_index_age': '1d', 'rollover_min_primary_shard_size': '30gb'},
    'std': {'retention_days': 30, 'rollover_min_index_age': '7d', 'rollover_min_primary_shard_size': '30gb'},
    'cold': {'retention_days': 90, 'rollover_min_index_age': '30d', 'rollover_min_primary_shard_size': '30gb'},
}

_INDEX_SEGMENT_RE = re.compile(r'[^a-z0-9_-]+')


def _safe_segment(value):
    """索引名段只允许小写字母/数字/下划线/连字符，避免非法索引名。"""
    segment = _INDEX_SEGMENT_RE.sub('-', str(value or '').strip().lower()).strip('-')
    return segment or 'unknown'


def build_index_name(index_prefix, environment_code, business_system_code, tier):
    """logs-<environment.code>-<business_system.code>-<tier>（§4.1）。

    服务/实例/主机/日志类型不进索引名，作为字段存储。
    """
    if tier not in RETENTION_TIERS:
        raise ValueError(f'未知保留档位: {tier}')
    return '-'.join([
        _safe_segment(index_prefix or 'logs'),
        _safe_segment(environment_code),
        _safe_segment(business_system_code),
        tier,
    ])


def build_index_template_name(index_prefix):
    return f'{_safe_segment(index_prefix or "logs")}-template'


def build_index_template_body(index_prefix):
    """data stream 模板，单分片、限制字段总数（§4.4 / §12 mapping 膨胀）。"""
    prefix = _safe_segment(index_prefix or 'logs')
    return {
        'index_patterns': [f'{prefix}-*'],
        'data_stream': {},
        'template': {
            'settings': {
                'number_of_shards': 1,
                'number_of_replicas': 0,
                'index.refresh_interval': '10s',
                'index.mapping.total_fields.limit': 2000,
            },
        },
    }


def build_ism_policy_name(index_prefix, tier):
    return f'{_safe_segment(index_prefix or "logs")}-{tier}-retention'


def build_ism_policy_body(index_prefix, tier):
    """按档位生成 ISM policy，经 ism_template 按索引名后缀自动挂载（§4.5）。"""
    config = RETENTION_TIERS.get(tier)
    if config is None:
        raise ValueError(f'未知保留档位: {tier}')
    prefix = _safe_segment(index_prefix or 'logs')
    return {
        'policy': {
            'description': f'{prefix} *-{tier} 索引保留 {config["retention_days"]} 天',
            'default_state': 'hot',
            'states': [
                {
                    'name': 'hot',
                    'actions': [
                        {
                            'rollover': {
                                'min_primary_shard_size': config['rollover_min_primary_shard_size'],
                                'min_index_age': config['rollover_min_index_age'],
                            },
                        },
                    ],
                    'transitions': [
                        {
                            'state_name': 'delete',
                            'conditions': {'min_index_age': f'{config["retention_days"]}d'},
                        },
                    ],
                },
                {'name': 'delete', 'actions': [{'delete': {}}]},
            ],
            'ism_template': [{'index_patterns': [f'{prefix}-*-{tier}'], 'priority': 100}],
        },
    }


def build_pipeline_name(application_code, log_name):
    """app-<application.code>-<log_name>：解析规则跟随应用类型，不随实例增长（§5.2）。"""
    return f'app-{_safe_segment(application_code)}-{_safe_segment(log_name)}'


def build_default_pipeline_body():
    """默认解析 pipeline：错误归一化 + 指纹 + 时间戳覆盖。

    - 归一化（IP/长数字/UUID）后生成 error_fingerprint，避免同类错误散成数百条（§5.3）。
    - 必须配置 on_failure，否则单条格式不符会拒掉整条日志（§5.3）。
    - date processor 用日志产生时间覆盖 @timestamp，否则记录的是采集时间（§12）。
    """
    return {
        'description': 'djadmin 默认日志解析：时间戳、错误归一化与指纹',
        'processors': [
            # 日志原文里由 Fluent Bit 注入的 log_time（tail 插件解析或采集时间）先落到 @timestamp。
            {
                'date': {
                    'field': 'log_time',
                    'target_field': '@timestamp',
                    'formats': ['ISO8601', 'yyyy-MM-dd HH:mm:ss,SSS', 'yyyy-MM-dd HH:mm:ss'],
                    'ignore_failure': True,
                },
            },
            {
                'gsub': {
                    'field': 'error_message',
                    'pattern': r'\d+\.\d+\.\d+\.\d+',
                    'replacement': '<IP>',
                    'target_field': 'error_template',
                    'ignore_missing': True,
                },
            },
            {
                'gsub': {
                    'field': 'error_template',
                    'pattern': r'\b\d{4,}\b',
                    'replacement': '<NUM>',
                    'ignore_missing': True,
                },
            },
            {
                'gsub': {
                    'field': 'error_template',
                    'pattern': r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}',
                    'replacement': '<UUID>',
                    'ignore_missing': True,
                },
            },
            {
                'fingerprint': {
                    'fields': ['error_type', 'error_template'],
                    'target_field': 'error_fingerprint',
                    'ignore_missing': True,
                },
            },
        ],
        'on_failure': [
            {'set': {'field': 'parse_error', 'value': '{{ _ingest.on_failure_message }}'}},
        ],
    }


def bootstrap_log_storage(cluster):
    """确保 index template 与三条 ISM policy 存在（§11 阶段 1 的自动化部分）。

    幂等：重复执行只会覆盖同名对象，不重复创建。返回写入的对象名清单。
    """
    client = OpenSearchClient(cluster)
    prefix = cluster.index_prefix or 'logs'

    template_name = build_index_template_name(prefix)
    client.put_index_template(template_name, build_index_template_body(prefix))

    policies = []
    for tier in RETENTION_TIERS:
        policy_name = build_ism_policy_name(prefix, tier)
        client.put_ism_policy(policy_name, build_ism_policy_body(prefix, tier))
        policies.append(policy_name)

    return {'index_template': template_name, 'ism_policies': policies}
