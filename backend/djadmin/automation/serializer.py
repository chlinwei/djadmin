import fnmatch
import re

from rest_framework import serializers
from rest_framework.serializers import ModelSerializer
from django.db.models import Q
from django.utils import timezone
import yaml

from assets.models import Host, HostGroup

from .models import (
    PlaybookTemplate,
    ShellScriptTemplate,
    AutomationTask,
    AutomationInventory,
    AnsibleExecutionJob,
    AutomationWorkflowTemplate,
    AutomationWorkflowRun,
)
from .workflow_runtime import get_workflow_runtime_status


WORKFLOW_NODE_TYPES = {'task', 'workflow'}
WORKFLOW_NODE_CONVERGENCE = {'any', 'all'}
WORKFLOW_EDGE_CONDITIONS = {'success', 'failure', 'always'}


def _build_workflow_refs_graph(start_workflow_id, workflow_ids, nodes_snapshot=None):
    """
    构建工作流引用图。
    
    Args:
        start_workflow_id: 当前工作流 ID
        workflow_ids: 被引用的工作流 ID 列表
        nodes_snapshot: 当前工作流的 nodes 快照（用于运行时）
    
    Returns:
        workflow_refs: {workflow_id -> set of referenced workflow_ids}
    """
    if not start_workflow_id or not workflow_ids:
        return {}

    all_workflows = AutomationWorkflowTemplate.objects.filter(
        id__in=set([start_workflow_id] + list(workflow_ids))
    ).values('id', 'nodes')
    
    workflow_refs = {}
    for wf in all_workflows:
        wf_id = wf['id']
        workflow_refs.setdefault(wf_id, set())
        
        # 对于当前工作流，优先使用 nodes_snapshot；否则从数据库查询
        nodes = nodes_snapshot if wf_id == start_workflow_id and nodes_snapshot else (wf.get('nodes') or [])
        for node in nodes:
            if isinstance(node, dict) and node.get('node_type') == 'workflow':
                ref_id = node.get('workflow_id')
                if ref_id and isinstance(ref_id, int):
                    workflow_refs[wf_id].add(ref_id)
    
    return workflow_refs


def _dfs_detect_cycle(start_wf_id, workflow_refs):
    """
    DFS 遍历检测循环，返回循环路径或 None。
    
    Args:
        start_wf_id: 起始工作流 ID
        workflow_refs: 工作流引用图
    
    Returns:
        (is_cycle, cycle_path_str) - is_cycle: bool, cycle_path_str: 循环路径描述或 None
    """
    visited = set()
    visiting = set()
    path_stack = []  # 记录访问路径用于错误信息
    
    def dfs(wf_id):
        if wf_id in visiting:
            # 找到环：构建环路径描述
            cycle_start_idx = next((i for i, x in enumerate(path_stack) if x == wf_id), 0)
            cycle_path = ' → '.join(str(x) for x in path_stack[cycle_start_idx:] + [wf_id])
            return cycle_path
        
        if wf_id in visited:
            return None
        
        visiting.add(wf_id)
        path_stack.append(wf_id)
        
        for ref_id in workflow_refs.get(wf_id, set()):
            result = dfs(ref_id)
            if result:  # 找到了循环
                return result
        
        path_stack.pop()
        visiting.discard(wf_id)
        visited.add(wf_id)
        return None
    
    if start_wf_id:
        cycle_path = dfs(start_wf_id)
        return (bool(cycle_path), cycle_path)
    
    return (False, None)


def _detect_workflow_cycle(start_workflow_id, referenced_workflow_ids, new_nodes=None):
    """
    检测跨工作流的循环引用（例如：workflow A → B → A）。
    用于序列化器验证阶段，抛出异常。
    """
    if not start_workflow_id or not referenced_workflow_ids:
        return

    workflow_refs = _build_workflow_refs_graph(start_workflow_id, referenced_workflow_ids, new_nodes)
    is_cycle, cycle_path = _dfs_detect_cycle(start_workflow_id, workflow_refs)
    
    if is_cycle:
        raise serializers.ValidationError(
            f'cross-workflow cycle detected: {cycle_path}'
        )


def check_workflow_cycle_at_runtime(workflow_id, nodes_snapshot):
    """
    在运行时检测工作流循环。
    
    Args:
        workflow_id: 要执行的工作流 ID
        nodes_snapshot: 工作流的 nodes 快照
    
    Returns:
        (is_cycle, error_message) - is_cycle: bool, error_message: 错误描述或 None
    """
    if not workflow_id or not nodes_snapshot:
        return (False, None)
    
    # 提取引用的工作流 ID
    referenced_workflow_ids = {
        node.get('workflow_id')
        for node in nodes_snapshot
        if isinstance(node, dict) and node.get('node_type') == 'workflow' and node.get('workflow_id')
    }
    
    if not referenced_workflow_ids:
        return (False, None)
    
    workflow_refs = _build_workflow_refs_graph(workflow_id, referenced_workflow_ids, nodes_snapshot)
    is_cycle, cycle_path = _dfs_detect_cycle(workflow_id, workflow_refs)
    
    if is_cycle:
        error_msg = f'Workflow execution blocked: circular reference detected - {cycle_path}'
        return (True, error_msg)
    
    return (False, None)


def validate_playbook_content_or_raise(content):
    content_text = str(content or '').strip()
    if not content_text:
        raise serializers.ValidationError('Playbook content cannot be empty')

    try:
        parsed = yaml.safe_load(content_text)
    except yaml.YAMLError as exc:
        raise serializers.ValidationError(f'Playbook YAML syntax error: {exc}') from exc

    if parsed is None:
        raise serializers.ValidationError('Playbook content cannot be empty')

    if not isinstance(parsed, list):
        raise serializers.ValidationError('Playbook YAML must be a list of plays')

    if not parsed:
        raise serializers.ValidationError('Playbook YAML must contain at least one play')

    for index, item in enumerate(parsed, start=1):
        if not isinstance(item, dict):
            raise serializers.ValidationError(f'Play #{index} must be an object')


class PlaybookTemplateSerializer(ModelSerializer):
    class Meta:
        model = PlaybookTemplate
        fields = '__all__'

    def validate_content(self, value):
        validate_playbook_content_or_raise(value)
        return value

    def create(self, validated_data):
        validated_data['create_time'] = timezone.now()
        return PlaybookTemplate.objects.create(**validated_data)


class ShellScriptTemplateSerializer(ModelSerializer):
    class Meta:
        model = ShellScriptTemplate
        fields = '__all__'

    def create(self, validated_data):
        validated_data['create_time'] = timezone.now()
        return ShellScriptTemplate.objects.create(**validated_data)


class AutomationTaskSerializer(ModelSerializer):
    template_name = serializers.SerializerMethodField()
    inventory_name = serializers.SerializerMethodField()
    selected_hosts = serializers.SerializerMethodField()
    resolved_hosts = serializers.SerializerMethodField()
    execution_scope_summary = serializers.SerializerMethodField()
    execution_scope_tree = serializers.SerializerMethodField()
    limit_preview_hosts = serializers.SerializerMethodField()
    limit_preview_total = serializers.SerializerMethodField()
    limit_preview_truncated = serializers.SerializerMethodField()
    limit_preview_limit = serializers.SerializerMethodField()

    class Meta:
        model = AutomationTask
        fields = '__all__'

    def get_template_name(self, obj):
        # 优先返回 playbook_template，如果没有则返回 shell_script_template
        if obj.playbook_template_id:
            return f'[Playbook] {obj.playbook_template.name}'
        elif obj.shell_script_template_id:
            return f'[ShellScript] {obj.shell_script_template.name}'
        return ''

    def get_inventory_name(self, obj):
        return obj.inventory.name if getattr(obj, 'inventory_id', None) else ''

    @staticmethod
    def _format_host_label(host_info):
        instance_name = host_info.get('instance_name') or host_info.get('name') or '-'
        ip = host_info.get('ip') or '-'
        if instance_name and str(instance_name) != str(ip):
            return f'{instance_name}({ip})'
        return str(ip)

    def _build_group_descendants(self, group_ids):
        if not group_ids:
            return set()

        id_set = set(group_ids)
        queue = list(group_ids)
        while queue:
            current = queue.pop(0)
            children = HostGroup.objects.filter(parent_id=current).values_list('id', flat=True)
            for child_id in children:
                if child_id not in id_set:
                    id_set.add(child_id)
                    queue.append(child_id)
        return id_set

    def _serialize_hosts(self, hosts):
        result = []
        for host in hosts:
            system = getattr(host, 'system', None)
            hostname = getattr(system, 'hostname', None) if system else None
            display_name = host.instance_name or host.ip or f'Host-{host.id}'
            result.append({
                'id': host.id,
                'name': display_name,
                'instance_name': host.instance_name,
                'hostname': hostname,
                'ip': host.ip,
                'group_id': host.group_id,
                'group_name': host.group.name if getattr(host, 'group', None) else '',
            })
        return result

    def _parse_limit_tokens(self, limit_text):
        tokens = [token.strip() for token in re.split(r'[\s,]+', str(limit_text or '').strip()) if token.strip()]
        include_tokens = []
        exclude_tokens = []
        for token in tokens:
            if token.startswith('!') and len(token) > 1:
                exclude_tokens.append(token[1:])
            else:
                include_tokens.append(token)
        return include_tokens, exclude_tokens

    def _build_group_path_map(self, group_ids):
        normalized_ids = [int(item) for item in group_ids if str(item).isdigit()]
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

        for group_id in normalized_ids:
            resolve_path(group_id)
        return cache

    def _match_limit_token(self, host_item, token):
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

        host_id_text = str(host_item.get('id') or '')
        host_name = str(host_item.get('name') or '').lower()
        host_ip = str(host_item.get('ip') or '').lower()
        group_name = str(host_item.get('group_name') or '').lower()
        group_path = str(host_item.get('group_path') or '').lower()

        if scope in ('host', 'hostname', 'name'):
            return fnmatch.fnmatch(host_name, pattern)
        if scope in ('id', 'host_id'):
            return fnmatch.fnmatch(host_id_text, pattern)
        if scope in ('path', 'group_path'):
            return fnmatch.fnmatch(group_path, pattern)
        if has_scope:
            return False

        return (
            fnmatch.fnmatch(host_id_text, pattern)
            or fnmatch.fnmatch(host_ip, pattern)
        )

    def _build_limit_preview(self, obj, preview_size=None):
        scope_payload = self._get_scope_payload(obj)
        resolved_hosts = scope_payload.get('resolved_hosts', [])
        if not isinstance(resolved_hosts, list) or len(resolved_hosts) == 0:
            return {'hosts': [], 'total': 0, 'truncated': False, 'limit': str(obj.default_limit or '').strip()}

        group_ids = [item.get('group_id') for item in resolved_hosts if item.get('group_id') is not None]
        group_path_map = self._build_group_path_map(group_ids)

        hosts_with_group_path = []
        for item in resolved_hosts:
            group_id = item.get('group_id')
            next_item = {**item}
            if group_id is not None and str(group_id).isdigit():
                next_item['group_path'] = group_path_map.get(int(group_id), '')
            else:
                next_item['group_path'] = ''
            hosts_with_group_path.append(next_item)

        normalized_limit = str(obj.default_limit or '').strip()
        matched_hosts = hosts_with_group_path
        if normalized_limit:
            include_tokens, exclude_tokens = self._parse_limit_tokens(normalized_limit)
            filtered = []
            for host_item in hosts_with_group_path:
                include_ok = True
                if include_tokens:
                    include_ok = any(self._match_limit_token(host_item, token) for token in include_tokens)
                exclude_hit = any(self._match_limit_token(host_item, token) for token in exclude_tokens)
                if include_ok and not exclude_hit:
                    filtered.append(host_item)
            matched_hosts = filtered

        # Keep preview deterministic and group-centric: group path first, then host display fields.
        matched_hosts = sorted(
            matched_hosts,
            key=lambda item: (
                str(item.get('group_path') or item.get('group_name') or '').lower(),
                str(item.get('name') or '').lower(),
                str(item.get('ip') or ''),
                int(item.get('id') or 0),
            ),
        )

        total = len(matched_hosts)
        preview_hosts = matched_hosts if preview_size is None else matched_hosts[:preview_size]
        return {
            'hosts': [
                {
                    'host_id': item.get('id'),
                    'host_name': item.get('name') or '-',
                    'host_ip': item.get('ip') or '-',
                    'group_path': item.get('group_path') or '',
                    'group_name': item.get('group_name') or '',
                }
                for item in preview_hosts
            ],
            'total': total,
            'truncated': total > len(preview_hosts),
            'limit': normalized_limit,
        }

    def _get_scope_payload(self, obj):
        source_inventory = getattr(obj, 'inventory', None)
        if source_inventory is not None:
            host_ids = [int(item) for item in (source_inventory.selected_host_ids or []) if str(item).isdigit()]
            group_ids = [int(item) for item in (source_inventory.selected_group_ids or []) if str(item).isdigit()]
            is_all_hosts = False
        else:
            # Inventory is now the single source of execution scope.
            host_ids = []
            group_ids = []
            is_all_hosts = False
        group_id_set = self._build_group_descendants(group_ids)

        filters = Q()
        if host_ids:
            filters |= Q(id__in=host_ids)
        if group_id_set:
            filters |= Q(group_id__in=list(group_id_set))

        resolved_hosts = []
        if filters:
            hosts = Host.objects.filter(ip__isnull=False).filter(filters).select_related('system').order_by('id').distinct()
            resolved_hosts = self._serialize_hosts(hosts)
        elif is_all_hosts:
            hosts = Host.objects.filter(ip__isnull=False).select_related('system').order_by('id')
            resolved_hosts = self._serialize_hosts(hosts)

        selected_hosts = []
        if host_ids:
            # For edit-form echo, keep selected hosts visible even if IP is empty.
            direct_hosts = Host.objects.filter(id__in=host_ids).select_related('system').order_by('id')
            selected_hosts = self._serialize_hosts(direct_hosts)

        return {
            'host_ids': host_ids,
            'group_ids': group_ids,
            'is_all_hosts': is_all_hosts,
            'group_id_set': group_id_set,
            'selected_hosts': selected_hosts,
            'resolved_hosts': resolved_hosts,
        }

    def _build_execution_scope_tree(self, scope_payload):
        group_ids = scope_payload['group_ids']
        group_id_set = scope_payload['group_id_set']
        resolved_hosts = scope_payload['resolved_hosts']
        selected_host_ids = set(scope_payload['host_ids'])

        if not group_ids and not resolved_hosts:
            return []

        group_records = list(HostGroup.objects.all().values('id', 'name', 'parent_id').order_by('id'))
        group_map = {item['id']: item for item in group_records}
        children_map = {}
        for item in group_records:
            children_map.setdefault(item['parent_id'], []).append(item['id'])

        grouped_hosts = {}
        standalone_hosts = []
        for host in resolved_hosts:
            host_node = {
                'key': f"host-{host['id']}",
                'title': self._format_host_label(host),
                'isLeaf': True,
                'node_type': 'host',
                'host_id': host['id'],
                'group_id': host.get('group_id'),
                'search': host.get('instance_name') or host.get('name') or host.get('hostname') or host.get('ip') or '',
            }
            group_id = host.get('group_id')
            if group_id in group_id_set:
                grouped_hosts.setdefault(group_id, []).append(host_node)
            elif host['id'] in selected_host_ids:
                standalone_hosts.append(host_node)

        def build_group_node(group_id):
            group_info = group_map.get(group_id)
            if group_info is None:
                return None, 0

            child_nodes = []
            host_count = 0
            for child_id in children_map.get(group_id, []):
                if child_id not in group_id_set:
                    continue
                child_node, child_count = build_group_node(child_id)
                if child_node is not None:
                    child_nodes.append(child_node)
                    host_count += child_count

            host_nodes = grouped_hosts.get(group_id, [])
            host_count += len(host_nodes)
            return {
                'key': f'group-{group_id}',
                'title': f"{group_info['name']} ({host_count})",
                'node_type': 'group',
                'group_id': group_id,
                'children': [*child_nodes, *host_nodes],
            }, host_count

        tree = []
        for group_id in group_ids:
            group_node, _ = build_group_node(group_id)
            if group_node is not None:
                tree.append(group_node)

        if standalone_hosts:
            tree.append({
                'key': 'direct-hosts',
                'title': f'直接选中主机 ({len(standalone_hosts)})',
                'node_type': 'virtual',
                'children': standalone_hosts,
            })

        return tree

    def _build_execution_scope_summary(self, scope_payload, tree):
        host_count = len(scope_payload['resolved_hosts'])
        group_count = len(scope_payload['group_ids'])
        direct_host_count = len(scope_payload['selected_hosts'])

        if scope_payload.get('is_all_hosts'):
            all_hosts_label = f'全部主机（{host_count}台）' if host_count > 0 else '全部主机（0台）'
            return {
                'label': all_hosts_label,
                'group_count': group_count,
                'host_count': host_count,
                'direct_host_count': direct_host_count,
                'has_tree': len(tree) > 0,
            }

        parts = []
        if group_count > 0:
            parts.append(f'{group_count}组')
        if host_count > 0:
            parts.append(f'{host_count}台主机')

        return {
            'label': ' / '.join(parts) if parts else '-',
            'group_count': group_count,
            'host_count': host_count,
            'direct_host_count': direct_host_count,
            'has_tree': len(tree) > 0,
        }

    def get_selected_hosts(self, obj):
        return self._get_scope_payload(obj)['selected_hosts']

    def get_execution_scope_tree(self, obj):
        scope_payload = self._get_scope_payload(obj)
        return self._build_execution_scope_tree(scope_payload)

    def get_execution_scope_summary(self, obj):
        scope_payload = self._get_scope_payload(obj)
        tree = self._build_execution_scope_tree(scope_payload)
        return self._build_execution_scope_summary(scope_payload, tree)

    def get_resolved_hosts(self, obj):
        return self._get_scope_payload(obj)['resolved_hosts']

    def get_limit_preview_hosts(self, obj):
        return self._build_limit_preview(obj)['hosts']

    def get_limit_preview_total(self, obj):
        return self._build_limit_preview(obj)['total']

    def get_limit_preview_truncated(self, obj):
        return self._build_limit_preview(obj)['truncated']

    def get_limit_preview_limit(self, obj):
        return self._build_limit_preview(obj)['limit']

    def validate_selected_host_ids(self, value):
        if not isinstance(value, list):
            raise serializers.ValidationError('selected_host_ids must be a list')
        return [int(item) for item in value if str(item).isdigit()]

    def validate_selected_group_ids(self, value):
        if not isinstance(value, list):
            raise serializers.ValidationError('selected_group_ids must be a list')
        return [int(item) for item in value if str(item).isdigit()]

    def validate_env_vars(self, value):
        if not isinstance(value, dict):
            raise serializers.ValidationError('env_vars must be an object')
        return value

    def validate_shell_parameters(self, value):
        if value is None:
            return ''
        return str(value).strip()

    def validate_default_limit(self, value):
        if value is None:
            return ''
        text = str(value).strip()
        if len(text) > 255:
            raise serializers.ValidationError('default_limit length must be <= 255')
        return text

    def validate(self, attrs):
        playbook_template = attrs.get('playbook_template')
        shell_script_template = attrs.get('shell_script_template')

        if self.instance is not None:
            if 'playbook_template' not in attrs:
                playbook_template = getattr(self.instance, 'playbook_template', None)
            if 'shell_script_template' not in attrs:
                shell_script_template = getattr(self.instance, 'shell_script_template', None)

        selected_count = int(playbook_template is not None) + int(shell_script_template is not None)
        if selected_count != 1:
            raise serializers.ValidationError('必须且只能选择一种模板（Playbook 或 Shell 脚本）')

        return attrs

    def create(self, validated_data):
        validated_data['create_time'] = timezone.now()
        return AutomationTask.objects.create(**validated_data)


class AnsibleExecutionJobSerializer(ModelSerializer):
    job_id = serializers.SerializerMethodField()
    template_name = serializers.SerializerMethodField()
    task_name = serializers.SerializerMethodField()

    class Meta:
        model = AnsibleExecutionJob
        fields = '__all__'

    def get_template_name(self, obj):
        return str(obj.template_name_snapshot or '').strip()

    def get_task_name(self, obj):
        # 优先使用快照字段（任务删除后仍能显示历史名称）
        if obj.task_name_snapshot:
            return obj.task_name_snapshot
        return obj.task.name if obj.task_id else ''

    def get_job_id(self, obj):
        # Expose a numeric, human-friendly execution ID for UI and API consumers.
        return obj.id

    def create(self, validated_data):
        validated_data['create_time'] = timezone.now()
        return AnsibleExecutionJob.objects.create(**validated_data)


class AutomationInventorySerializer(ModelSerializer):
    scope_summary = serializers.SerializerMethodField()
    health_status = serializers.SerializerMethodField()
    resolved_host_count = serializers.SerializerMethodField()

    class Meta:
        model = AutomationInventory
        fields = '__all__'

    def _parse_scope(self, obj):
        group_ids = [int(item) for item in (obj.selected_group_ids or []) if str(item).isdigit()]
        host_ids = [int(item) for item in (obj.selected_host_ids or []) if str(item).isdigit()]
        return group_ids, host_ids

    def _evaluate_scope(self, obj):
        cache_attr = '_scope_eval_cache'
        cache = getattr(self, cache_attr, {})
        if obj.id in cache:
            return cache[obj.id]

        group_ids, host_ids = self._parse_scope(obj)
        is_empty_scope = len(group_ids) == 0 and len(host_ids) == 0
        existing_group_ids = set(HostGroup.objects.filter(id__in=group_ids).values_list('id', flat=True))
        missing_group_ids = sorted(set(group_ids) - existing_group_ids)

        if is_empty_scope:
            resolved_host_count = 0
        else:
            conditions = Q()
            if host_ids:
                conditions |= Q(id__in=host_ids)
            descendant_ids = self._build_group_descendants(group_ids)
            if descendant_ids:
                conditions |= Q(group_id__in=list(descendant_ids))
            resolved_host_count = Host.objects.filter(ip__isnull=False).filter(conditions).distinct().count() if conditions else 0

        result = {
            'group_ids': group_ids,
            'host_ids': host_ids,
            'is_empty_scope': is_empty_scope,
            'missing_group_ids': missing_group_ids,
            'resolved_host_count': resolved_host_count,
        }
        cache[obj.id] = result
        setattr(self, cache_attr, cache)
        return result

    def _build_group_descendants(self, group_ids):
        if not group_ids:
            return set()

        id_set = set(group_ids)
        queue = list(group_ids)
        while queue:
            current = queue.pop(0)
            children = HostGroup.objects.filter(parent_id=current).values_list('id', flat=True)
            for child_id in children:
                if child_id not in id_set:
                    id_set.add(child_id)
                    queue.append(child_id)
        return id_set

    def _resolved_host_count(self, obj):
        return self._evaluate_scope(obj)['resolved_host_count']

    def get_scope_summary(self, obj):
        scope = self._evaluate_scope(obj)
        group_ids = scope['group_ids']
        resolved_host_count = scope['resolved_host_count']
        return {
            'label': f"{len(group_ids)}组 / {resolved_host_count}台主机",
            'group_count': len(group_ids),
            'host_count': resolved_host_count,
            'is_empty_scope': scope['is_empty_scope'],
        }

    def get_health_status(self, obj):
        scope = self._evaluate_scope(obj)
        missing_group_ids = scope['missing_group_ids']
        resolved_host_count = scope['resolved_host_count']

        if missing_group_ids:
            return {
                'status': 'invalid',
                'label': '范围失效',
                'message': f'存在已删除主机组: {", ".join(str(item) for item in missing_group_ids)}',
            }

        if resolved_host_count == 0:
            return {
                'status': 'empty',
                'label': '空范围',
                'message': '当前 Inventory 无可用主机',
            }

        return {
            'status': 'healthy',
            'label': '正常',
            'message': f'当前可执行主机 {resolved_host_count} 台',
        }

    def get_resolved_host_count(self, obj):
        return self._resolved_host_count(obj)

    def validate_selected_host_ids(self, value):
        if not isinstance(value, list):
            raise serializers.ValidationError('selected_host_ids must be a list')
        return [int(item) for item in value if str(item).isdigit()]

    def validate_selected_group_ids(self, value):
        if not isinstance(value, list):
            raise serializers.ValidationError('selected_group_ids must be a list')
        return [int(item) for item in value if str(item).isdigit()]

    def validate(self, attrs):
        host_ids = attrs.get('selected_host_ids')
        group_ids = attrs.get('selected_group_ids')

        if host_ids is None and self.instance is not None:
            host_ids = self.instance.selected_host_ids
        if group_ids is None and self.instance is not None:
            group_ids = self.instance.selected_group_ids

        host_ids = host_ids if isinstance(host_ids, list) else []
        group_ids = group_ids if isinstance(group_ids, list) else []

        if len(host_ids) == 0 and len(group_ids) == 0:
            raise serializers.ValidationError('请至少选择一个主机组后再保存 Inventory')

        return attrs

    def create(self, validated_data):
        validated_data['create_time'] = timezone.now()
        return AutomationInventory.objects.create(**validated_data)


def validate_workflow_graph_or_raise(nodes, edges, entry_node_key, allow_empty=False, current_workflow_id=None):
    # 这里只做模板结构合法性校验（字段/引用/DAG），不做运行时递归链拦截。
    if not isinstance(nodes, list):
        raise serializers.ValidationError('nodes must be a list')
    if not isinstance(edges, list):
        raise serializers.ValidationError('edges must be a list')

    if len(nodes) == 0:
        if len(edges) > 0:
            raise serializers.ValidationError('edges must be empty when nodes is empty')
        if allow_empty:
            return [], [], ''
        raise serializers.ValidationError('nodes must be a non-empty list')

    node_map = {}
    normalized_nodes = []
    task_ids = []
    workflow_ids = []
    for index, node in enumerate(nodes, start=1):
        if not isinstance(node, dict):
            raise serializers.ValidationError(f'nodes[{index}] must be an object')

        node_name = str(node.get('name') or '').strip()
        if not node_name:
            raise serializers.ValidationError(f'nodes[{index}] name is required')

        node_key = str(node.get('key') or '').strip()
        if not node_key:
            node_key = f'node-{index}'
        if node_key in node_map:
            raise serializers.ValidationError(f'duplicate node key: {node_key}')

        node_type = str(node.get('node_type') or '').strip().lower()
        if node_type not in WORKFLOW_NODE_TYPES:
            raise serializers.ValidationError(f'nodes[{index}] node_type must be one of {sorted(WORKFLOW_NODE_TYPES)}')

        normalized_node = {
            'key': node_key,
            'name': node_name,
            'node_type': node_type,
            'convergence': 'any',
        }

        convergence = str(node.get('convergence') or 'any').strip().lower()
        if convergence not in WORKFLOW_NODE_CONVERGENCE:
            raise serializers.ValidationError(
                f'nodes[{index}] convergence must be one of {sorted(WORKFLOW_NODE_CONVERGENCE)}'
            )
        normalized_node['convergence'] = convergence

        x_value = node.get('x')
        y_value = node.get('y')
        if isinstance(x_value, (int, float)):
            normalized_node['x'] = float(x_value)
        if isinstance(y_value, (int, float)):
            normalized_node['y'] = float(y_value)

        if node_type == 'task':
            task_id_raw = node.get('task_id')
            if task_id_raw is None or not str(task_id_raw).isdigit():
                raise serializers.ValidationError(f'nodes[{index}] task_id is required for task node')
            task_id = int(task_id_raw)
            task_ids.append(task_id)
            normalized_node['task_id'] = task_id
        else:
            workflow_id_raw = node.get('workflow_id')
            if workflow_id_raw is None or not str(workflow_id_raw).isdigit():
                raise serializers.ValidationError(f'nodes[{index}] workflow_id is required for workflow node')
            workflow_id = int(workflow_id_raw)
            workflow_ids.append(workflow_id)
            normalized_node['workflow_id'] = workflow_id

        node_map[node_key] = normalized_node
        normalized_nodes.append(normalized_node)

    if task_ids:
        existing_task_ids = set(AutomationTask.objects.filter(id__in=task_ids).values_list('id', flat=True))
        missing_task_ids = sorted(set(task_ids) - existing_task_ids)
        if missing_task_ids:
            raise serializers.ValidationError(f'task nodes reference missing tasks: {missing_task_ids}')

    if workflow_ids:
        existing_workflow_ids = set(AutomationWorkflowTemplate.objects.filter(id__in=workflow_ids).values_list('id', flat=True))
        missing_workflow_ids = sorted(set(workflow_ids) - existing_workflow_ids)
        if missing_workflow_ids:
            raise serializers.ValidationError(f'workflow nodes reference missing workflows: {missing_workflow_ids}')

    normalized_edges = []
    graph = {node_key: [] for node_key in node_map.keys()}
    for index, edge in enumerate(edges, start=1):
        if not isinstance(edge, dict):
            raise serializers.ValidationError(f'edges[{index}] must be an object')

        source_key = str(edge.get('source_key') or '').strip()
        target_key = str(edge.get('target_key') or '').strip()
        condition = str(edge.get('condition') or '').strip().lower() or 'success'
        if source_key not in node_map:
            raise serializers.ValidationError(f'edges[{index}] source_key does not exist: {source_key}')
        if target_key not in node_map:
            raise serializers.ValidationError(f'edges[{index}] target_key does not exist: {target_key}')
        if condition not in WORKFLOW_EDGE_CONDITIONS:
            raise serializers.ValidationError(
                f'edges[{index}] condition must be one of {sorted(WORKFLOW_EDGE_CONDITIONS)}'
            )

        normalized_edge = {
            'source_key': source_key,
            'target_key': target_key,
            'condition': condition,
        }
        normalized_edges.append(normalized_edge)
        graph[source_key].append(target_key)

    # 标准 DFS 染色法检测环：0=未访问，1=访问中，2=访问完成。
    color = {node_key: 0 for node_key in node_map.keys()}

    def _visit(node_key):
        if color[node_key] == 1:
            return True
        if color[node_key] == 2:
            return False

        color[node_key] = 1
        for child in graph.get(node_key, []):
            if _visit(child):
                return True
        color[node_key] = 2
        return False

    for node_key in node_map.keys():
        if color[node_key] == 0 and _visit(node_key):
            raise serializers.ValidationError('workflow graph must be a DAG; cycle detected')

    # Entry node is no longer used by workflow runtime; keep it empty for backward-compatible storage.
    return normalized_nodes, normalized_edges, ''


class AutomationWorkflowTemplateSerializer(ModelSerializer):
    node_count = serializers.SerializerMethodField()
    edge_count = serializers.SerializerMethodField()
    default_inventory_name = serializers.SerializerMethodField()
    execution_scope_summary = serializers.SerializerMethodField()

    class Meta:
        model = AutomationWorkflowTemplate
        fields = '__all__'
        extra_kwargs = {
            'entry_node_key': {'required': False, 'allow_blank': True},
        }

    def get_node_count(self, obj):
        return len(obj.nodes) if isinstance(obj.nodes, list) else 0

    def get_edge_count(self, obj):
        return len(obj.edges) if isinstance(obj.edges, list) else 0

    def get_default_inventory_name(self, obj):
        if getattr(obj, 'default_inventory_id', None) and getattr(obj, 'default_inventory', None) is not None:
            return obj.default_inventory.name or ''
        return ''

    def _build_group_descendants(self, group_ids):
        if not group_ids:
            return set()

        id_set = set(group_ids)
        queue = list(group_ids)
        while queue:
            current = queue.pop(0)
            children = HostGroup.objects.filter(parent_id=current).values_list('id', flat=True)
            for child_id in children:
                if child_id not in id_set:
                    id_set.add(child_id)
                    queue.append(child_id)
        return id_set

    def get_execution_scope_summary(self, obj):
        inventory = getattr(obj, 'default_inventory', None)
        if inventory is None:
            return {
                'label': '未设置 Inventory',
                'group_count': 0,
                'host_count': 0,
                'has_inventory': False,
                'is_empty_scope': True,
                'limit': str(getattr(obj, 'default_limit', '') or '').strip(),
            }

        group_ids = [int(item) for item in (inventory.selected_group_ids or []) if str(item).isdigit()]
        host_ids = [int(item) for item in (inventory.selected_host_ids or []) if str(item).isdigit()]
        is_empty_scope = len(group_ids) == 0 and len(host_ids) == 0

        host_count = 0
        if not is_empty_scope:
            conditions = Q()
            if host_ids:
                conditions |= Q(id__in=host_ids)
            descendant_ids = self._build_group_descendants(group_ids)
            if descendant_ids:
                conditions |= Q(group_id__in=list(descendant_ids))
            host_count = Host.objects.filter(ip__isnull=False).filter(conditions).distinct().count() if conditions else 0

        if is_empty_scope:
            label = 'Inventory 无主机'
        else:
            label = f'{len(group_ids)}组 / {host_count}台主机'

        return {
            'label': label,
            'group_count': len(group_ids),
            'host_count': host_count,
            'has_inventory': True,
            'is_empty_scope': is_empty_scope,
            'limit': str(getattr(obj, 'default_limit', '') or '').strip(),
        }

    def validate_default_extra_vars(self, value):
        if not isinstance(value, dict):
            raise serializers.ValidationError('default_extra_vars must be an object')
        return value

    def validate_default_limit(self, value):
        return str(value or '').strip()

    def validate(self, attrs):
        source_nodes = attrs.get('nodes', self.instance.nodes if self.instance is not None else None)
        source_edges = attrs.get('edges', self.instance.edges if self.instance is not None else None)
        source_entry = ''
        # 创建和编辑时都允许空节点，执行时才需要节点
        allow_empty_graph = True

        normalized_nodes, normalized_edges, normalized_entry = validate_workflow_graph_or_raise(
            source_nodes,
            source_edges,
            source_entry,
            allow_empty=allow_empty_graph,
            current_workflow_id=self.instance.id if self.instance is not None else None,
        )
        attrs['nodes'] = normalized_nodes
        attrs['edges'] = normalized_edges
        attrs['entry_node_key'] = ''
        return attrs

    def create(self, validated_data):
        validated_data['create_time'] = timezone.now()
        return AutomationWorkflowTemplate.objects.create(**validated_data)


class AutomationWorkflowRunSerializer(ModelSerializer):
    workflow_name = serializers.SerializerMethodField()
    workflow_nodes = serializers.SerializerMethodField()
    workflow_edges = serializers.SerializerMethodField()
    node_results_runtime = serializers.SerializerMethodField()
    runtime_status = serializers.SerializerMethodField()
    error_message = serializers.SerializerMethodField()
    runtime_hosts_preview = serializers.SerializerMethodField()
    runtime_hosts_total = serializers.SerializerMethodField()

    class Meta:
        model = AutomationWorkflowRun
        fields = '__all__'

    def get_error_message(self, instance):
        """如果运行失败，从 result_summary 中提取错误信息。"""
        if instance.status != AutomationWorkflowRun.Status.FAILED:
            return None
        if not isinstance(instance.result_summary, dict):
            return None
        return instance.result_summary.get('error')

    def _collect_runtime_hosts_preview(self, obj):
        node_results = obj.node_results if isinstance(obj.node_results, list) else []
        job_ids = [int(item['job_id']) for item in node_results if isinstance(item, dict) and str(item.get('job_id', '')).isdigit()]
        if not job_ids:
            return []

        rows = AnsibleExecutionJob.objects.filter(id__in=list(set(job_ids))).values('inventory_snapshot')
        host_map = {}

        for row in rows:
            snapshot = row.get('inventory_snapshot')
            if not isinstance(snapshot, dict):
                continue
            hosts = snapshot.get('hosts')
            if not isinstance(hosts, list):
                continue

            for host in hosts:
                if not isinstance(host, dict):
                    continue
                host_id = int(host['host_id']) if str(host.get('host_id', '')).isdigit() else None
                host_name = str(host.get('host_name') or '').strip()
                host_ip = str(host.get('host_ip') or '').strip()
                group_path = str(host.get('group_path') or '').strip()
                group_name = str(host.get('group_name') or '').strip()

                dedup_key = str(host_id) if host_id is not None else f"{host_name}|{host_ip}|{group_path}|{group_name}"
                if dedup_key in host_map:
                    continue

                host_map[dedup_key] = {
                    'host_id': host_id,
                    'host_name': host_name or host_ip or '-',
                    'host_ip': host_ip or '-',
                    'group_path': group_path,
                    'group_name': group_name,
                }

        return sorted(
            host_map.values(),
            key=lambda item: (
                str(item.get('group_path') or item.get('group_name') or '').lower(),
                str(item.get('host_name') or '').lower(),
                str(item.get('host_ip') or ''),
            ),
        )

    def get_runtime_hosts_preview(self, obj):
        return self._collect_runtime_hosts_preview(obj)

    def get_runtime_hosts_total(self, obj):
        return len(self._collect_runtime_hosts_preview(obj))

    def to_representation(self, instance):
        data = super().to_representation(instance)
        # result_summary is internal runtime bookkeeping (snapshots/dispatch meta),
        # no longer exposed as a user-facing "摘要" field.
        data.pop('result_summary', None)
        data.pop('node_results', None)
        return data

    @staticmethod
    def _enrich_node_reference_names(nodes):
        if not isinstance(nodes, list) or len(nodes) == 0:
            return []

        task_ids = set()
        workflow_ids = set()
        normalized_nodes = []

        for item in nodes:
            if not isinstance(item, dict):
                continue
            copied = dict(item)
            task_id = copied.get('task_id')
            workflow_id = copied.get('workflow_id')
            if str(task_id).isdigit():
                task_ids.add(int(task_id))
            if str(workflow_id).isdigit():
                workflow_ids.add(int(workflow_id))
            normalized_nodes.append(copied)

        task_name_map = {}
        if task_ids:
            rows = AutomationTask.objects.filter(id__in=list(task_ids)).values('id', 'name')
            task_name_map = {int(row['id']): str(row.get('name') or '') for row in rows}

        workflow_name_map = {}
        if workflow_ids:
            rows = AutomationWorkflowTemplate.objects.filter(id__in=list(workflow_ids)).values('id', 'name')
            workflow_name_map = {int(row['id']): str(row.get('name') or '') for row in rows}

        for item in normalized_nodes:
            task_id = item.get('task_id')
            if str(task_id).isdigit() and not str(item.get('task_name') or '').strip():
                item['task_name'] = task_name_map.get(int(task_id), '')

            workflow_id = item.get('workflow_id')
            if str(workflow_id).isdigit() and not str(item.get('workflow_name') or '').strip():
                item['workflow_name'] = workflow_name_map.get(int(workflow_id), '')

        return normalized_nodes

    @staticmethod
    def _resolve_snapshot_list(obj, summary_key: str, model_attr: str, field_name: str):
        summary = obj.result_summary if isinstance(obj.result_summary, dict) else {}
        snapshot_items = summary.get(summary_key, [])
        if isinstance(snapshot_items, list):
            return snapshot_items

        model_obj = getattr(obj, model_attr, None)
        items = getattr(model_obj, field_name, []) if model_obj is not None else []
        return items if isinstance(items, list) else []

    def get_workflow_name(self, obj):
        return obj.workflow_name_snapshot or ''

    def get_workflow_nodes(self, obj):
        nodes = self._resolve_snapshot_list(obj, 'workflow_nodes_snapshot', 'workflow', 'nodes')
        return self._enrich_node_reference_names(nodes)

    def get_workflow_edges(self, obj):
        return self._resolve_snapshot_list(obj, 'workflow_edges_snapshot', 'workflow', 'edges')

    def get_node_results_runtime(self, obj):
        source_results = obj.node_results if isinstance(obj.node_results, list) else []
        if not source_results:
            return []

        now = timezone.now()
        job_ids = []
        child_run_ids = []
        normalized_results = []
        for item in source_results:
            if not isinstance(item, dict):
                continue
            copied = dict(item)
            job_id = copied.get('job_id')
            if str(job_id).isdigit():
                job_ids.append(int(job_id))
            child_run_id = copied.get('child_run_id')
            if str(child_run_id).isdigit():
                child_run_ids.append(int(child_run_id))
            normalized_results.append(copied)

        job_status_map = {}
        job_time_map = {}
        job_meta_map = {}
        if job_ids:
            rows = list(AnsibleExecutionJob.objects.filter(id__in=list(set(job_ids))).values(
                'id', 'status', 'start_time', 'end_time', 'duration_seconds',
                'task_id', 'task__playbook_template_id', 'task__shell_script_template_id',
                'task_name_snapshot', 'template_name_snapshot'
            ))
            job_status_map = {int(row['id']): str(row.get('status') or '').lower() for row in rows}
            job_time_map = {int(row['id']): row for row in rows}
            job_meta_map = {int(row['id']): row for row in rows}

        child_run_status_map = {}
        child_run_time_map = {}
        if child_run_ids:
            rows = list(AutomationWorkflowRun.objects.filter(id__in=list(set(child_run_ids))).values(
                'id', 'status', 'start_time', 'end_time', 'duration_seconds'
            ))
            child_run_status_map = {int(row['id']): str(row.get('status') or '').lower() for row in rows}
            child_run_time_map = {int(row['id']): row for row in rows}

        def _resolve_duration_seconds(row: dict | None, live_status: str) -> float | None:
            if not isinstance(row, dict):
                return None

            duration_value = row.get('duration_seconds')
            if duration_value is not None:
                try:
                    return max(float(duration_value), 0.0)
                except (TypeError, ValueError):
                    return None

            start_time = row.get('start_time')
            end_time = row.get('end_time')
            if start_time is None:
                return None

            if live_status in {'running', 'pending', 'queued'}:
                end_time = now
            if end_time is None:
                return None

            try:
                return max((end_time - start_time).total_seconds(), 0.0)
            except Exception:
                return None

        for item in normalized_results:
            item['job_task_id'] = item.get('job_task_id') if str(item.get('job_task_id', '')).isdigit() else item.get('task_id')
            item['job_template_id'] = None
            item['job_task_name_snapshot'] = str(
                item.get('job_task_name_snapshot')
                or item.get('task_name_snapshot')
                or ''
            ).strip()
            item['job_template_name_snapshot'] = str(
                item.get('job_template_name_snapshot')
                or item.get('template_name_snapshot')
                or ''
            ).strip()

            job_id = item.get('job_id')
            if not str(job_id).isdigit():
                continue

            live_status = job_status_map.get(int(job_id), '')
            if not live_status:
                continue

            if live_status in {'pending', 'running', 'success', 'failed', 'cancelled'}:
                item['status'] = live_status
                item['duration_seconds'] = _resolve_duration_seconds(job_time_map.get(int(job_id)), live_status)
                job_meta = job_meta_map.get(int(job_id), {})
                item['job_task_id'] = int(job_meta['task_id']) if str(job_meta.get('task_id', '')).isdigit() else None
                # AnsibleExecutionJob 不再直接关联 template，模板 ID 通过 task 的 playbook/shell 字段反查。
                playbook_template_id = job_meta.get('task__playbook_template_id')
                shell_template_id = job_meta.get('task__shell_script_template_id')
                if str(playbook_template_id or '').isdigit():
                    item['job_template_id'] = int(playbook_template_id)
                elif str(shell_template_id or '').isdigit():
                    item['job_template_id'] = int(shell_template_id)
                else:
                    item['job_template_id'] = None
                item['job_task_name_snapshot'] = str(job_meta.get('task_name_snapshot') or '').strip()
                item['job_template_name_snapshot'] = str(job_meta.get('template_name_snapshot') or '').strip()

        for item in normalized_results:
            child_run_id = item.get('child_run_id')
            if not str(child_run_id).isdigit():
                continue

            live_status = child_run_status_map.get(int(child_run_id), '')
            if not live_status:
                continue

            if live_status in {'pending', 'running', 'success', 'failed'}:
                item['status'] = live_status if live_status != 'pending' else 'running'
                item['duration_seconds'] = _resolve_duration_seconds(child_run_time_map.get(int(child_run_id)), live_status)

        return normalized_results

    def get_runtime_status(self, obj):
        node_results = self.get_node_results_runtime(obj)
        workflow_edges = self.get_workflow_edges(obj)
        return get_workflow_runtime_status(node_results, workflow_edges, fallback_status=obj.status)
