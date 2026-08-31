"""Ansible 风格 limit 表达式的解析与匹配。

view_helpers（真实 inventory 快照过滤）与 serializer（Task 详情预览）过去各写了一份几乎一样的实现，
这里合并成一份共享逻辑；两处主机字典的字段名不同，通过 match_limit_token 的关键字参数区分。
"""
import fnmatch
import re

from assets.models import HostGroup


def parse_limit_tokens(limit_text):
    """把逗号/空白分隔的 limit 表达式拆成 (include_tokens, exclude_tokens)，'!' 前缀表示排除。"""
    tokens = [token.strip() for token in re.split(r'[\s,]+', str(limit_text or '').strip()) if token.strip()]
    include_tokens = []
    exclude_tokens = []
    for token in tokens:
        if token.startswith('!') and len(token) > 1:
            exclude_tokens.append(token[1:])
        else:
            include_tokens.append(token)
    return include_tokens, exclude_tokens


def match_limit_token(host_item, token, *, id_field='host_id', name_field='host_name', ip_field='host_ip'):
    """匹配单个 limit token；host_item 的字段名由调用方通过关键字参数指定。"""
    raw_token = str(token or '').strip().lower()
    if not raw_token:
        return False

    scope = ''
    has_scope = False
    pattern = raw_token
    if ':' in raw_token:
        has_scope = True
        scope, pattern = raw_token.split(':', 1)
        scope = scope.strip()
        pattern = pattern.strip()
        if not pattern:
            return False

    host_id_text = str(host_item.get(id_field) or '')
    host_name = str(host_item.get(name_field) or '').lower()
    host_ip = str(host_item.get(ip_field) or '').lower()
    group_path = str(host_item.get('group_path') or '').lower()

    if scope in ('host', 'hostname', 'name'):
        return fnmatch.fnmatch(host_name, pattern)
    if scope in ('id', 'host_id'):
        return fnmatch.fnmatch(host_id_text, pattern)
    if scope in ('path', 'group_path'):
        return fnmatch.fnmatch(group_path, pattern)
    if has_scope:
        return False

    return fnmatch.fnmatch(host_id_text, pattern) or fnmatch.fnmatch(host_ip, pattern)


def build_group_path_map(group_ids):
    """把分组 id 列表解析成 {id: 'parent/child'} 路径映射。"""
    normalized_ids = []
    for item in group_ids:
        try:
            normalized_ids.append(int(item))
        except (TypeError, ValueError):
            continue
    if not normalized_ids:
        return {}

    group_rows = list(HostGroup.objects.all().values('id', 'name', 'parent_id'))
    group_lookup = {int(item['id']): item for item in group_rows if item.get('id') is not None}
    cache = {}

    def resolve_path(group_id):
        if group_id in cache:
            return cache[group_id]
        row = group_lookup.get(group_id)
        if not row:
            cache[group_id] = ''
            return ''

        name = str(row.get('name') or '').strip()
        parent_id_raw = row.get('parent_id')
        parent_id = int(parent_id_raw) if isinstance(parent_id_raw, int) else None
        if parent_id and parent_id != group_id:
            parent_path = resolve_path(parent_id)
            cache[group_id] = f'{parent_path}/{name}' if parent_path else name
        else:
            cache[group_id] = name
        return cache[group_id]

    for gid in normalized_ids:
        resolve_path(gid)
    return cache
