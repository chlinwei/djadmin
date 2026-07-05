from datetime import datetime
import fnmatch
import re

from rest_framework import serializers
from rest_framework.serializers import ModelSerializer
from django.db.models import Q
import yaml

from assets.models import Host, HostGroup

from .models import PlaybookTemplate, AutomationTask, AutomationInventory, AnsibleExecutionJob, AnsibleExecutionTarget


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
        validated_data['create_time'] = datetime.now().date()
        return PlaybookTemplate.objects.create(**validated_data)


class AnsibleExecutionTargetSerializer(ModelSerializer):
    class Meta:
        model = AnsibleExecutionTarget
        fields = '__all__'


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
        return obj.template.name if obj.template_id else ''

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
        pattern = str(token or '').strip().lower()
        if not pattern:
            return False

        host_id_text = str(host_item.get('id') or '')
        host_name = str(host_item.get('name') or '').lower()
        host_ip = str(host_item.get('ip') or '').lower()
        group_name = str(host_item.get('group_name') or '').lower()
        group_path = str(host_item.get('group_path') or '').lower()

        return (
            fnmatch.fnmatch(host_id_text, pattern)
            or fnmatch.fnmatch(host_name, pattern)
            or fnmatch.fnmatch(host_ip, pattern)
            or fnmatch.fnmatch(group_name, pattern)
            or fnmatch.fnmatch(group_path, pattern)
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
            host_ids = [int(item) for item in (obj.selected_host_ids or []) if str(item).isdigit()]
            group_ids = [int(item) for item in (obj.selected_group_ids or []) if str(item).isdigit()]
            is_all_hosts = len(host_ids) == 0 and len(group_ids) == 0
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

    def validate_default_limit(self, value):
        if value is None:
            return ''
        text = str(value).strip()
        if len(text) > 255:
            raise serializers.ValidationError('default_limit length must be <= 255')
        return text

    def validate(self, attrs):
        host_ids = attrs.get('selected_host_ids')
        group_ids = attrs.get('selected_group_ids')

        # Support partial update by falling back to current instance values.
        if host_ids is None and self.instance is not None:
            host_ids = self.instance.selected_host_ids
        if group_ids is None and self.instance is not None:
            group_ids = self.instance.selected_group_ids

        host_ids = host_ids if isinstance(host_ids, list) else []
        group_ids = group_ids if isinstance(group_ids, list) else []

        return attrs

    def create(self, validated_data):
        validated_data['create_time'] = datetime.now().date()
        return AutomationTask.objects.create(**validated_data)


class AnsibleExecutionJobSerializer(ModelSerializer):
    job_id = serializers.SerializerMethodField()
    targets = AnsibleExecutionTargetSerializer(many=True, read_only=True)
    template_name = serializers.SerializerMethodField()
    task_name = serializers.SerializerMethodField()

    class Meta:
        model = AnsibleExecutionJob
        fields = '__all__'

    def get_template_name(self, obj):
        return obj.template.name if obj.template_id else ''

    def get_task_name(self, obj):
        return obj.task.name if obj.task_id else ''

    def get_job_id(self, obj):
        # Expose a numeric, human-friendly execution ID for UI and API consumers.
        return obj.id

    def create(self, validated_data):
        validated_data['create_time'] = datetime.now().date()
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
        validated_data['create_time'] = datetime.now().date()
        return AutomationInventory.objects.create(**validated_data)
