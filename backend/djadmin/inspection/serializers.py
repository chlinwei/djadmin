from rest_framework import serializers

from .models import (
    InspectionCheck,
    InspectionExecution,
    InspectionGroup,
    InspectionResult,
    InspectionTargetExecution,
    InspectionTask,
)
from .service import host_scope_name

# 主机组巡检只能解析 ${HOST_IP}/${HOST_NAME}，以下应用上下文变量会原样进入 shell 并被展开为空。
APPLICATION_VARIABLES = ('${APP_HOME}', '${RUN_USER}', '${INSTANCE_NAME}', '${APPLICATION_VERSION}', '${SERVICE_NAME}')
APPLICATION_VARIABLE_ERROR = '主机组巡检不能使用应用上下文变量，请使用 ${HOST_IP} 或 ${HOST_NAME}'


def _normalize_id_list(value, field_name):
    if not isinstance(value, list):
        raise serializers.ValidationError(f'{field_name} 必须是数组')
    normalized = []
    for item in value:
        if not str(item).lstrip('-').isdigit() or int(item) <= 0:
            raise serializers.ValidationError(f'{field_name} 只能包含正整数 ID')
        if int(item) not in normalized:
            normalized.append(int(item))
    return normalized


def contains_application_variable(value):
    if isinstance(value, dict):
        return any(contains_application_variable(item) for item in value.values())
    if isinstance(value, list):
        return any(contains_application_variable(item) for item in value)
    return isinstance(value, str) and any(variable in value for variable in APPLICATION_VARIABLES)


class InspectionCheckSerializer(serializers.ModelSerializer):
    class Meta:
        model = InspectionCheck
        fields = ['id', 'name', 'executor', 'config', 'severity', 'enabled', 'order']

    def validate(self, attrs):
        executor = attrs.get('executor')
        config = attrs.get('config') or {}
        if executor == InspectionCheck.Executor.SHELL and not str(config.get('command') or '').strip():
            raise serializers.ValidationError({'config': 'Shell 命令不能为空'})
        if executor == InspectionCheck.Executor.SCHEMA_VALIDATE:
            schema_type = str(config.get('schema_type') or '')
            document_type = str(config.get('document_type') or '')
            allowed_document_types = {
                'json_schema': {'json', 'yaml', 'toml', 'ini', 'properties'},
                'schematron': {'xml'},
                'regexp': {'text'},
            }
            if not str(config.get('path') or '').strip():
                raise serializers.ValidationError({'config': '待校验文件路径不能为空'})
            if schema_type not in allowed_document_types:
                raise serializers.ValidationError({'config': 'Schema 类型无效'})
            if document_type not in allowed_document_types[schema_type]:
                raise serializers.ValidationError({'config': '文档类型与 Schema 类型不匹配'})
            if not str(config.get('schema_content') or '').strip():
                raise serializers.ValidationError({'config': 'Schema 内容不能为空'})
        if executor == InspectionCheck.Executor.HTTP:
            url = str(config.get('url') or '').strip()
            if not url.startswith(('http://', 'https://')):
                raise serializers.ValidationError({'config': 'HTTP URL 必须以 http:// 或 https:// 开头'})
        if executor == InspectionCheck.Executor.TCP:
            try:
                port = int(config.get('port') or 0)
            except (TypeError, ValueError):
                port = 0
            if port < 1 or port > 65535:
                raise serializers.ValidationError({'config': 'TCP 端口无效'})
        return attrs


class InspectionGroupSerializer(serializers.ModelSerializer):
    checks = InspectionCheckSerializer(many=True, required=False)

    class Meta:
        model = InspectionGroup
        fields = ['id', 'name', 'scope', 'description', 'enabled', 'checks', 'create_time', 'update_time']

    def validate(self, attrs):
        checks = attrs.get('checks')
        if checks is None and self.instance is not None:
            checks = list(self.instance.checks.values('name', 'executor', 'config', 'enabled', 'order'))
        checks = checks or []
        allowed = set(InspectionCheck.Executor.values)
        invalid = [check.get('name') for check in checks if check.get('executor') not in allowed]
        if invalid:
            raise serializers.ValidationError({'checks': f'检查项执行器无效: {", ".join(invalid)}'})

        scope = attrs.get('scope', getattr(self.instance, 'scope', None))
        # 跨目标族切换会让已绑定任务的目标字段整体失效（如只填了逻辑服务却改成主机组）。
        if self.instance is not None and self.instance.tasks.exists():
            was_host = self.instance.scope == InspectionGroup.Scope.PER_HOST
            now_host = scope == InspectionGroup.Scope.PER_HOST
            if was_host != now_host:
                raise serializers.ValidationError({
                    'scope': '巡检组已被任务引用，不能在“逻辑服务”与“主机组”之间切换范围，请新建巡检组',
                })

        if scope == InspectionGroup.Scope.PER_HOST:
            offending = [
                check.get('name')
                for check in checks
                if check.get('enabled', True) and contains_application_variable(check.get('config'))
            ]
            if offending:
                raise serializers.ValidationError({
                    'checks': f'{APPLICATION_VARIABLE_ERROR}；请修改检查项: {", ".join(filter(None, offending))}',
                })
        return attrs

    def _replace_checks(self, group, checks):
        group.checks.all().delete()
        InspectionCheck.objects.bulk_create([InspectionCheck(group=group, **check) for check in checks])

    def create(self, validated_data):
        checks = validated_data.pop('checks', [])
        group = super().create(validated_data)
        self._replace_checks(group, checks)
        return group

    def update(self, instance, validated_data):
        checks = validated_data.pop('checks', None)
        group = super().update(instance, validated_data)
        if checks is not None:
            self._replace_checks(group, checks)
        return group


class InspectionTaskSerializer(serializers.ModelSerializer):
    group_name = serializers.CharField(source='group.name', read_only=True)
    scope = serializers.CharField(source='group.scope', read_only=True)
    logical_service_name = serializers.CharField(source='logical_service.name', read_only=True)
    target_type = serializers.CharField(read_only=True)
    target_name = serializers.SerializerMethodField()

    class Meta:
        model = InspectionTask
        fields = [
            'id', 'name', 'inspection_name', 'group', 'group_name', 'scope', 'target_type', 'target_name',
            'logical_service', 'logical_service_name', 'selected_host_ids',
            'concurrency', 'timeout_seconds', 'cron_expression', 'next_run_time', 'last_run_time',
            'enabled', 'create_time', 'update_time',
        ]
        read_only_fields = ['next_run_time', 'last_run_time']

    def validate_cron_expression(self, value):
        cron_text = str(value or '').strip()
        if not cron_text:
            return ''
        # 定时巡检与任务中心共用同一套 cron 解析，避免两处语义不一致。
        from scheduler_manager import validate_cron_expression

        valid, error = validate_cron_expression(cron_text)
        if not valid:
            raise serializers.ValidationError(error)
        return cron_text

    def validate_selected_host_ids(self, value):
        return _normalize_id_list(value, 'selected_host_ids')

    def get_target_name(self, obj):
        if obj.target_type == InspectionTask.TargetType.HOST_GROUP:
            return host_scope_name(obj.selected_host_ids)
        return obj.logical_service.name if obj.logical_service else ''

    def validate(self, attrs):
        instance = self.instance
        cron_expression = str(attrs.get('cron_expression', getattr(instance, 'cron_expression', '')) or '').strip()
        inspection_name = str(attrs.get('inspection_name', getattr(instance, 'inspection_name', '')) or '').strip()
        if cron_expression and not inspection_name:
            raise serializers.ValidationError({'inspection_name': '配置定时计划时必须填写巡检名称'})
        group = attrs.get('group', getattr(instance, 'group', None))
        logical_service = attrs.get('logical_service', getattr(instance, 'logical_service', None))
        host_ids = attrs.get('selected_host_ids', getattr(instance, 'selected_host_ids', None) or [])

        if group is not None and group.scope == InspectionGroup.Scope.PER_HOST:
            if not host_ids:
                raise serializers.ValidationError({'selected_host_ids': '请勾选主机'})
            attrs['logical_service'] = None
        else:
            if logical_service is None:
                raise serializers.ValidationError({'logical_service': '请选择逻辑服务'})
            attrs['selected_host_ids'] = []
        return attrs

    def validate_group(self, group):
        if not group.enabled:
            raise serializers.ValidationError('巡检组已禁用')
        if not group.checks.filter(enabled=True).exists():
            raise serializers.ValidationError('巡检组没有启用的检查项')
        return group


class InspectionResultSerializer(serializers.ModelSerializer):
    class Meta:
        model = InspectionResult
        fields = ['id', 'check_key', 'check_type', 'name', 'status', 'severity', 'expected_value', 'actual_value', 'message']


class InspectionTargetExecutionSerializer(serializers.ModelSerializer):
    results = InspectionResultSerializer(many=True, read_only=True)

    class Meta:
        model = InspectionTargetExecution
        fields = [
            'id', 'deployment', 'host', 'target_name', 'host_id_snapshot', 'host_ip_snapshot',
            'agent_id_snapshot', 'status', 'passed', 'error_message', 'raw_result',
            'start_time', 'end_time', 'results',
        ]


class InspectionExecutionSerializer(serializers.ModelSerializer):
    task_name = serializers.CharField(source='task.name', read_only=True)
    targets = InspectionTargetExecutionSerializer(many=True, read_only=True)

    class Meta:
        model = InspectionExecution
        fields = [
            'id', 'task', 'task_name', 'status', 'trigger_type', 'task_snapshot', 'group_snapshot',
            'service_snapshot', 'target_snapshot', 'summary', 'requested_username',
            'start_time', 'end_time', 'targets', 'create_time',
        ]


class InspectionExecutionListSerializer(serializers.ModelSerializer):
    task_name = serializers.CharField(source='task.name', read_only=True)
    target_name = serializers.SerializerMethodField()

    class Meta:
        model = InspectionExecution
        fields = [
            'id', 'task', 'task_name', 'target_name', 'status', 'trigger_type', 'summary',
            'requested_username', 'start_time', 'end_time', 'create_time',
        ]

    def get_target_name(self, obj):
        return str((obj.service_snapshot or {}).get('name') or '')