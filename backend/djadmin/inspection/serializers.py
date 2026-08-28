from rest_framework import serializers

from .models import (
    InspectionCheck,
    InspectionExecution,
    InspectionGroup,
    InspectionResult,
    InspectionTargetExecution,
    InspectionTask,
)


class InspectionCheckSerializer(serializers.ModelSerializer):
    class Meta:
        model = InspectionCheck
        fields = ['id', 'name', 'executor', 'config', 'enabled', 'order']

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
            raise serializers.ValidationError({'checks': f'检查项执行器与执行范围不匹配: {", ".join(invalid)}'})
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
    host_group_name = serializers.CharField(source='host_group.name', read_only=True)
    target_name = serializers.SerializerMethodField()

    class Meta:
        model = InspectionTask
        fields = [
            'id', 'name', 'group', 'group_name', 'scope', 'target_type', 'target_name',
            'logical_service', 'logical_service_name', 'host_group', 'host_group_name',
            'concurrency', 'timeout_seconds', 'enabled', 'create_time', 'update_time',
        ]

    def get_target_name(self, obj):
        if obj.target_type == InspectionTask.TargetType.HOST_GROUP:
            return obj.host_group.name if obj.host_group else ''
        return obj.logical_service.name if obj.logical_service else ''

    @staticmethod
    def _contains_application_variable(value):
        application_variables = ('${APP_HOME}', '${RUN_USER}', '${INSTANCE_NAME}', '${APPLICATION_VERSION}', '${SERVICE_NAME}')
        if isinstance(value, dict):
            return any(InspectionTaskSerializer._contains_application_variable(item) for item in value.values())
        if isinstance(value, list):
            return any(InspectionTaskSerializer._contains_application_variable(item) for item in value)
        return isinstance(value, str) and any(variable in value for variable in application_variables)

    def validate(self, attrs):
        instance = self.instance
        group = attrs.get('group', getattr(instance, 'group', None))
        target_type = attrs.get('target_type', getattr(instance, 'target_type', InspectionTask.TargetType.LOGICAL_SERVICE))
        logical_service = attrs.get('logical_service', getattr(instance, 'logical_service', None))
        host_group = attrs.get('host_group', getattr(instance, 'host_group', None))

        if target_type == InspectionTask.TargetType.HOST_GROUP:
            if group and group.scope != InspectionGroup.Scope.PER_DEPLOYMENT:
                raise serializers.ValidationError({'target_type': '主机组仅支持 Agent Shell 巡检组'})
            if host_group is None:
                raise serializers.ValidationError({'host_group': '请选择主机组'})
            if group and any(self._contains_application_variable(check.config) for check in group.checks.filter(enabled=True)):
                raise serializers.ValidationError({'host_group': '主机组巡检不能使用应用上下文变量，请使用 ${HOST_IP} 或 ${HOST_NAME}'})
            attrs['logical_service'] = None
        else:
            if logical_service is None:
                raise serializers.ValidationError({'logical_service': '请选择逻辑服务'})
            attrs['host_group'] = None
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
        fields = ['id', 'check_key', 'check_type', 'name', 'status', 'expected_value', 'actual_value', 'message']


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
            'id', 'task', 'task_name', 'status', 'task_snapshot', 'group_snapshot',
            'service_snapshot', 'target_snapshot', 'summary', 'requested_username',
            'start_time', 'end_time', 'targets', 'create_time',
        ]