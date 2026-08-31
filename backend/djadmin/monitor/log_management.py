"""日志存储（OpenSearch）侧的管理面构建器。

对应《日志采集架构文档》第 4/5 节：索引命名、index template、ISM policy、
ingest pipeline 的默认体。全部为纯函数，不触碰网络，便于单测；
bootstrap_log_storage 是唯一执行写入的编排入口。
"""
import re

from django.db import transaction

from .log_schema import STANDARD_LOG_FIELDS
from .opensearch_client import OpenSearchClient, OpenSearchError

_INDEX_SEGMENT_RE = re.compile(r'[^a-z0-9_-]+')


def _safe_segment(value):
    """索引名段只允许小写字母/数字/下划线/连字符，避免非法索引名。"""
    segment = _INDEX_SEGMENT_RE.sub('-', str(value or '').strip().lower()).strip('-')
    return segment or 'unknown'


def build_index_name(index_prefix, project_code, environment_code, business_system_code, tier_code):
    """logs-<project.code>-<environment.code>-<business_system.code>-<tier.code>（§4.1）。

    服务/实例/主机/日志类型不进索引名，作为字段存储。
    project 和 tier 都直接决定索引归属，缺失时必须报错，不能静默写成 unknown 而造出难以清理的 data stream。
    """
    if not str(project_code or '').strip():
        raise ValueError('缺少所属项目')
    if not str(tier_code or '').strip():
        raise ValueError('缺少保留档位')
    return '-'.join([
        _safe_segment(index_prefix or 'logs'),
        _safe_segment(project_code),
        _safe_segment(environment_code),
        _safe_segment(business_system_code),
        _safe_segment(tier_code),
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
            'mappings': {
                # 标准字段之外一律不再自动建 mapping，应用差异字段必须写进 app_fields。
                'dynamic': False,
                'properties': STANDARD_LOG_FIELDS,
            },
        },
    }


def build_ism_policy_name(index_prefix, tier_code):
    return f'{_safe_segment(index_prefix or "logs")}-{_safe_segment(tier_code)}-retention'


def build_ism_policy_body(index_prefix, tier):
    """按档位记录生成 ISM policy，经 ism_template 按索引名后缀自动挂载（§4.5）。

    tier 是 monitor.LogRetentionTier 实例：档位已改为用户可维护的数据。
    """
    prefix = _safe_segment(index_prefix or 'logs')
    code = _safe_segment(tier.code)
    return {
        'policy': {
            'description': (
                f'{prefix} *-{code} 索引保留 {tier.retention_days} 天'
                f'（预估 {tier.daily_size_gb}GB/天，合计 {tier.estimated_total_gb}GB）'
            ),
            'default_state': 'hot',
            'states': [
                {
                    'name': 'hot',
                    'actions': [
                        {
                            'rollover': {
                                'min_primary_shard_size': tier.rollover_min_primary_shard_size,
                                'min_index_age': tier.rollover_min_index_age,
                            },
                        },
                    ],
                    'transitions': [
                        {
                            'state_name': 'delete',
                            'conditions': {'min_index_age': f'{tier.retention_days}d'},
                        },
                    ],
                },
                {'name': 'delete', 'actions': [{'delete': {}}]},
            ],
            'ism_template': [{'index_patterns': [f'{prefix}-*-{code}'], 'priority': 100}],
        },
    }


def _upsert_ism_policy(client, name, body):
    """覆盖已存在的 ISM policy 必须带乐观锁参数，否则 OpenSearch 直接返回 409。"""
    try:
        existing = client.get_ism_policy(name)
    except OpenSearchError:
        existing = None
    params = None
    if existing and existing.get('_seq_no') is not None:
        params = {'if_seq_no': existing['_seq_no'], 'if_primary_term': existing['_primary_term']}
    client.put_ism_policy(name, body, params=params)


def build_pipeline_name(application_code, log_name):
    """app-<application.code>-<log_name>：解析规则跟随应用类型，不随实例增长（§5.2）。"""
    return f'app-{_safe_segment(application_code)}-{_safe_segment(log_name)}'


def bootstrap_log_storage(cluster):
    """确保 index template 与各保留档位的 ISM policy 存在（§11 阶段 1 的自动化部分）。

    幂等：重复执行只会覆盖同名对象，不重复创建。返回写入的对象名清单。
    档位新增/修改后需重新执行本方法，策略才会下发到集群。
    """
    from .models import LogRetentionTier

    client = OpenSearchClient(cluster)
    prefix = cluster.index_prefix or 'logs'

    template_name = build_index_template_name(prefix)
    client.put_index_template(template_name, build_index_template_body(prefix))

    policies = []
    for tier in LogRetentionTier.objects.filter(enabled=True):
        policy_name = build_ism_policy_name(prefix, tier.code)
        _upsert_ism_policy(client, policy_name, build_ism_policy_body(prefix, tier))
        policies.append(policy_name)

    return {'index_template': template_name, 'ism_policies': policies}


def sync_log_storage_quietly(cluster):
    """下发模板与保留策略，失败只返回原因、不抛异常。

    索引模板是代码派生物而非用户配置，必须自动跟随代码版本；但集群暂时不可达
    不应阻塞集群保存或数据库迁移，因此这里吞掉异常，由调用方决定如何提示。
    """
    if not cluster or not cluster.enabled:
        return ''
    try:
        bootstrap_log_storage(cluster)
    except OpenSearchError as exc:
        return str(exc)[:500]
    return ''


def enqueue_log_storage_sync(cluster_id):
    """在事务提交后入队，避免 worker 读取到尚未提交的集群配置。"""
    from .models import OpenSearchCluster
    from .tasks import sync_log_storage

    cluster = OpenSearchCluster.objects.filter(pk=cluster_id, enabled=True).first()
    if cluster is None:
        return
    cluster.storage_sync_status = OpenSearchCluster.StorageSyncStatus.PENDING
    cluster.storage_sync_error = ''
    cluster.storage_sync_time = None
    cluster.save(update_fields=['storage_sync_status', 'storage_sync_error', 'storage_sync_time', 'update_time'])

    # Celery Task.delay 的运行时 API 正确，Pylance 对 shared_task 返回对象的存根误判为列表。
    transaction.on_commit(lambda: sync_log_storage.delay(cluster_id))  # type: ignore[operator]
