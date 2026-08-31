"""日志文档契约：标准字段定义与越界字段校验。

与 log_management.py 分开：那边构建 OpenSearch 管理面对象（索引名/模板/ISM），
这里只定义「一条日志文档长什么样」，被索引模板、规则保存校验、在线调试三处消费。

标准字段的判据是「语义是否通用」，不是「推导方式是否相同」：error_fingerprint 在
Java 里由异常类型加归一化模板算出、在 MySQL 里可取消息前缀算出，推导不同但含义一致，
所以是标准字段。error_type、error_message、stack_trace 这类只有报错日志才有的字段
（access log、INFO 日志根本没有），连同应用私有字段（logger、error_code 等）进 app_fields。

app_fields 是 flat_object：子字段可以 term 查询，但**不支持聚合**（返回空桶且不报错），
因此任何需要聚合的字段都必须留在标准字段里。
"""
import re

APP_FIELDS_KEY = 'app_fields'

STANDARD_LOG_FIELDS = {
    '@timestamp': {'type': 'date'},
    'message': {'type': 'text'},
    'project': {'type': 'keyword'},
    'business_system': {'type': 'keyword'},
    'environment': {'type': 'keyword'},
    'service': {'type': 'keyword'},
    'application': {'type': 'keyword'},
    'instance': {'type': 'keyword'},
    # 主机 IP 允许为空串，用 keyword 而非 ip 类型，避免空值直接写入失败。
    'host_ip': {'type': 'keyword'},
    'log_name': {'type': 'keyword'},
    'log_path': {'type': 'keyword'},
    'log_level': {'type': 'keyword'},
    'log_time': {'type': 'keyword'},
    'log_message': {'type': 'text'},
    # 错误归组标识：各应用推导方式不同但语义一致，insight/error-* 靠它做 terms 聚合，
    # 而 app_fields 是 flat_object、子字段聚合会返回空桶，所以必须留在顶层。
    'error_fingerprint': {'type': 'keyword'},
    APP_FIELDS_KEY: {'type': 'flat_object'},
}

# Ingest 运行期的内部字段，不落文档，校验时跳过。
_PIPELINE_INTERNAL_FIELDS = {'_index', '_id', '_type', '_routing', '_ingest'}
_GROK_NAMED_PATTERN_RE = re.compile(r'%\{[A-Z0-9_]+:([^:}]+)(?::[a-z]+)?\}')
_GROK_NAMED_GROUP_RE = re.compile(r'\(\?P?<([^>]+)>')


def _is_allowed_field(field):
    root = field.split('.', 1)[0]
    return root in _PIPELINE_INTERNAL_FIELDS or root == APP_FIELDS_KEY or field in STANDARD_LOG_FIELDS


def _collect_processor_fields(processor, written, removed):
    """静态解析单个 processor 写出/删除的目标字段名。"""
    for name, config in processor.items():
        if not isinstance(config, dict):
            continue
        if name == 'remove':
            target = config.get('field')
            removed.update([target] if isinstance(target, str) else (target or []))
            continue
        if name == 'grok':
            for pattern in config.get('patterns') or []:
                written.update(_GROK_NAMED_PATTERN_RE.findall(str(pattern)))
                written.update(_GROK_NAMED_GROUP_RE.findall(str(pattern)))
        elif name == 'rename':
            # rename 是移动语义：源字段改完就不存在了，否则临时字段会被误判成越界。
            if isinstance(config.get('field'), str):
                removed.add(config['field'])
            if config.get('target_field'):
                written.add(config['target_field'])
        elif name == 'date':
            written.add(config.get('target_field') or '@timestamp')
        elif config.get('target_field'):
            written.add(config['target_field'])
        elif name in {'set', 'append', 'rename', 'convert', 'trim', 'lowercase', 'uppercase', 'gsub', 'split', 'join'}:
            if isinstance(config.get('field'), str):
                written.add(config['field'])
        # on_failure 里的处理器同样会写字段，必须一并递归。
        for nested in config.get('on_failure') or []:
            if isinstance(nested, dict):
                _collect_processor_fields(nested, written, removed)


def find_non_standard_pipeline_fields(pipeline_body):
    """返回 pipeline 会写出、但既不是标准字段也不在 app_fields 下的字段名。

    索引模板是 dynamic=false，越界字段会被静默丢弃（写入不报错但无法检索），
    因此必须在规则保存时拦截，而不是等日志进了 OpenSearch 才发现。
    """
    written, removed = set(), set()
    processors = list((pipeline_body or {}).get('processors') or [])
    processors.extend((pipeline_body or {}).get('on_failure') or [])
    for processor in processors:
        if isinstance(processor, dict):
            _collect_processor_fields(processor, written, removed)

    violations = {
        field for field in (str(item or '').strip() for item in written - removed)
        if field and not _is_allowed_field(field)
    }
    return sorted(violations)


def find_non_standard_document_fields(simulate_result):
    """从 pipeline 模拟输出的真实文档里找出越界字段。

    静态解析覆盖不到 script 处理器等动态写字段的场景，调试时用实际输出兜底。
    """
    violations = set()
    for item in (simulate_result or {}).get('docs') or []:
        source = ((item or {}).get('doc') or {}).get('_source') or {}
        violations.update(
            field for field in (str(key or '').strip() for key in source)
            if field and not _is_allowed_field(field)
        )
    return sorted(violations)
