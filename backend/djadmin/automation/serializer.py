from typing import Any

from rest_framework import serializers
from rest_framework.serializers import ModelSerializer
from django.utils import timezone
import yaml

from assets.models import Host, HostGroup

from .limit_utils import build_group_path_map, match_limit_token, parse_limit_tokens
from .models import (
    PlaybookTemplate,
    AutomationTask,
    AutomationInventory,
    AutomationExecutionJob,
)

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
        if obj.playbook_template_id:
            return f'[Playbook] {obj.playbook_template.name}'
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

    def _match_limit_token(self, host_item, token):
        return match_limit_token(host_item, token, id_field='id', name_field='name', ip_field='ip')

    def _build_limit_preview(self, obj, preview_size=None):
        scope_payload = self._get_scope_payload(obj)
        resolved_hosts = scope_payload.get('resolved_hosts', [])
        if not isinstance(resolved_hosts, list) or len(resolved_hosts) == 0:
            return {'hosts': [], 'total': 0, 'truncated': False, 'limit': str(obj.default_limit or '').strip()}

        group_ids = [item.get('group_id') for item in resolved_hosts if item.get('group_id') is not None]
        group_path_map = build_group_path_map(group_ids)

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
            include_tokens, exclude_tokens = parse_limit_tokens(normalized_limit)
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
        # Inventory is now the single source of execution scope, and it stores fixed host ids only.
        host_ids = (
            [int(item) for item in (source_inventory.selected_host_ids or []) if str(item).isdigit()]
            if source_inventory is not None else []
        )

        resolved_hosts = []
        if host_ids:
            hosts = Host.objects.filter(id__in=host_ids, ip__isnull=False).select_related('system').order_by('id')
            resolved_hosts = self._serialize_hosts(hosts)

        selected_hosts = []
        if host_ids:
            # For edit-form echo, keep selected hosts visible even if IP is empty.
            direct_hosts = Host.objects.filter(id__in=host_ids).select_related('system').order_by('id')
            selected_hosts = self._serialize_hosts(direct_hosts)

        # 分组信息只用于展示，从已选主机反推所属分组及其祖先链。
        group_id_set, group_ids = self._resolve_scope_group_tree(resolved_hosts)

        return {
            'host_ids': host_ids,
            'group_ids': group_ids,
            'is_all_hosts': False,
            'group_id_set': group_id_set,
            'selected_hosts': selected_hosts,
            'resolved_hosts': resolved_hosts,
        }

    @staticmethod
    def _resolve_scope_group_tree(resolved_hosts):
        """返回 (范围内涉及的全部分组集合, 树根分组列表)：根 = 其父分组不在集合内的分组。"""
        leaf_group_ids = {
            int(host['group_id']) for host in resolved_hosts
            if isinstance(host, dict) and host.get('group_id') is not None
        }
        if not leaf_group_ids:
            return set(), []

        parent_map = {
            int(item['id']): item['parent_id']
            for item in HostGroup.objects.all().values('id', 'parent_id')
        }
        group_id_set = set()
        for group_id in leaf_group_ids:
            current = group_id
            while current is not None and current not in group_id_set:
                group_id_set.add(current)
                current = parent_map.get(current)

        roots = sorted(
            group_id for group_id in group_id_set
            if parent_map.get(group_id) not in group_id_set
        )
        return group_id_set, roots

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

    def validate_env_vars(self, value):
        if not isinstance(value, dict):
            raise serializers.ValidationError('env_vars must be an object')
        return value

    def validate_default_limit(self, value):
        if value is None:
            return ''
        text = str(value).strip()
        if len(text) > 255:
            raise serializers.ValidationError('default_limit length must be <= 255')
        return text

    def validate(self, attrs):
        playbook_template = attrs.get('playbook_template')

        if self.instance is not None:
            if 'playbook_template' not in attrs:
                playbook_template = getattr(self.instance, 'playbook_template', None)

        if playbook_template is None:
            raise serializers.ValidationError('自动化任务必须绑定 Playbook 模板')

        return attrs

    def create(self, validated_data):
        validated_data['create_time'] = timezone.now()
        return AutomationTask.objects.create(**validated_data)


class AutomationExecutionJobSerializer(ModelSerializer):
    job_id = serializers.SerializerMethodField()
    template_name = serializers.SerializerMethodField()
    task_name = serializers.SerializerMethodField()

    class Meta:
        model = AutomationExecutionJob
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
        return AutomationExecutionJob.objects.create(**validated_data)


class AutomationInventorySerializer(ModelSerializer):
    scope_summary = serializers.SerializerMethodField()
    health_status = serializers.SerializerMethodField()
    resolved_host_count = serializers.SerializerMethodField()

    class Meta:
        model = AutomationInventory
        fields = '__all__'

    def _parse_scope(self, obj):
        return [int(item) for item in (obj.selected_host_ids or []) if str(item).isdigit()]

    def _evaluate_scope(self, obj):
        cache_attr = '_scope_eval_cache'
        cache = getattr(self, cache_attr, {})
        if obj.id in cache:
            return cache[obj.id]

        host_ids = self._parse_scope(obj)
        existing_host_ids = set(Host.objects.filter(id__in=host_ids).values_list('id', flat=True))
        missing_host_ids = sorted(set(host_ids) - existing_host_ids)
        group_ids = sorted({
            int(group_id)
            for group_id in Host.objects.filter(id__in=host_ids, group_id__isnull=False).values_list('group_id', flat=True)
        })
        resolved_host_count = Host.objects.filter(id__in=host_ids, ip__isnull=False).count()

        result = {
            'group_ids': group_ids,
            'host_ids': host_ids,
            'is_empty_scope': len(host_ids) == 0,
            'missing_host_ids': missing_host_ids,
            'resolved_host_count': resolved_host_count,
        }
        cache[obj.id] = result
        setattr(self, cache_attr, cache)
        return result

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
        missing_host_ids = scope['missing_host_ids']
        resolved_host_count = scope['resolved_host_count']

        if missing_host_ids:
            return {
                'status': 'invalid',
                'label': '范围失效',
                'message': f'存在已删除主机: {", ".join(str(item) for item in missing_host_ids)}',
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

    def validate(self, attrs):
        host_ids = attrs.get('selected_host_ids')
        if host_ids is None and self.instance is not None:
            host_ids = self.instance.selected_host_ids

        if not isinstance(host_ids, list) or len(host_ids) == 0:
            raise serializers.ValidationError('请至少选择一台主机后再保存 Inventory')

        return attrs

    def create(self, validated_data):
        validated_data['create_time'] = timezone.now()
        return AutomationInventory.objects.create(**validated_data)


