from rest_framework.serializers import ModelSerializer
from datetime import datetime, timedelta
import json
import re
import xml.etree.ElementTree as ET
from jsonschema import Draft202012Validator
from jsonschema.exceptions import SchemaError
from .models import *
from .application_variables import APP_HOME_VARIABLE, RUN_USER_VARIABLE, ApplicationVariableError, resolve_application_variables
from .credential_crypto import encrypt_secret
from rest_framework import serializers
from django.utils import timezone
from django.db import transaction



# host user
class CredentialSerializer(ModelSerializer):
    class Meta:
        model = Credential
        fields = '__all__'

    @staticmethod
    def _encrypt_password_if_provided(validated_data):
        if 'password' in validated_data:
            validated_data['password'] = encrypt_secret(validated_data.get('password'))
    
    # 创建
    def create(self, validated_data):
        self._encrypt_password_if_provided(validated_data)
        validated_data["create_time"] = timezone.now()
        data = Credential.objects.create(**validated_data)
        return data

    def update(self, instance, validated_data):
        self._encrypt_password_if_provided(validated_data)
        return super().update(instance, validated_data)

    def validate_private_key(self, value):
        if value and len(value) > 8192:
            raise serializers.ValidationError("私钥长度超过限制")
        return value
class ApplicationVersionSerializer(ModelSerializer):
    application_name = serializers.CharField(source='application.name', read_only=True)

    class Meta:
        model = ApplicationVersion
        fields = '__all__'

    def create(self, validated_data):
        validated_data["create_time"] = timezone.now()
        return ApplicationVersion.objects.create(**validated_data)


class ApplicationPortSerializer(ModelSerializer):
    class Meta:
        model = ApplicationPort
        exclude = ['deployment_template']


class ApplicationPathSerializer(ModelSerializer):
    class Meta:
        model = ApplicationPath
        exclude = ['deployment_template']


class ApplicationConfigFileSerializer(ModelSerializer):
    class Meta:
        model = ApplicationConfigFile
        exclude = ['deployment_template']


class ApplicationLogDefinitionSerializer(ModelSerializer):
    class Meta:
        model = ApplicationLogDefinition
        exclude = ['deployment_template']


class ApplicationControlActionSerializer(ModelSerializer):
    class Meta:
        model = ApplicationControlAction
        exclude = ['deployment_template']


class DockerControlConfigSerializer(ModelSerializer):
    class Meta:
        model = DockerControlConfig
        exclude = ['deployment_template']


class DockerComposeControlConfigSerializer(ModelSerializer):
    class Meta:
        model = DockerComposeControlConfig
        exclude = ['deployment_template']


class ApplicationBaselineCheckSerializer(ModelSerializer):
    document_type = serializers.ChoiceField(choices=ApplicationBaselineCheck.DocumentType.choices, required=True)
    schema_type = serializers.ChoiceField(choices=ApplicationBaselineCheck.SchemaType.choices, required=True)
    schema_version = serializers.CharField(required=True)
    SCHEMA_TYPES_BY_DOCUMENT = {
        ApplicationBaselineCheck.DocumentType.XML: {
            ApplicationBaselineCheck.SchemaType.SCHEMATRON,
        },
        ApplicationBaselineCheck.DocumentType.JSON: {ApplicationBaselineCheck.SchemaType.JSON_SCHEMA},
        ApplicationBaselineCheck.DocumentType.YAML: {ApplicationBaselineCheck.SchemaType.JSON_SCHEMA},
        ApplicationBaselineCheck.DocumentType.INI: {ApplicationBaselineCheck.SchemaType.JSON_SCHEMA},
        ApplicationBaselineCheck.DocumentType.TOML: {ApplicationBaselineCheck.SchemaType.JSON_SCHEMA},
        ApplicationBaselineCheck.DocumentType.PROPERTIES: {ApplicationBaselineCheck.SchemaType.JSON_SCHEMA},
        ApplicationBaselineCheck.DocumentType.TEXT: {ApplicationBaselineCheck.SchemaType.REGEXP},
    }
    SCHEMA_VERSIONS = {
        ApplicationBaselineCheck.SchemaType.SCHEMATRON: 'iso',
        ApplicationBaselineCheck.SchemaType.JSON_SCHEMA: '2020-12',
        ApplicationBaselineCheck.SchemaType.REGEXP: 're2',
    }

    class Meta:
        model = ApplicationBaselineCheck
        exclude = ['application']

    def validate_schema_content(self, content):
        if not str(content or '').strip():
            raise serializers.ValidationError('Schema 内容不能为空')
        return content

    def validate(self, attrs):
        attrs = super().validate(attrs)
        document_type = attrs.get('document_type', getattr(self.instance, 'document_type', None))
        schema_type = attrs.get('schema_type', getattr(self.instance, 'schema_type', None))
        schema_version = attrs.get('schema_version', getattr(self.instance, 'schema_version', None))
        schema_content = attrs.get('schema_content', getattr(self.instance, 'schema_content', ''))

        if schema_type not in self.SCHEMA_TYPES_BY_DOCUMENT.get(document_type, set()):
            raise serializers.ValidationError({'schema_type': 'Schema 类型与文档类型不匹配'})
        expected_version = self.SCHEMA_VERSIONS[schema_type]
        if schema_version != expected_version:
            raise serializers.ValidationError({'schema_version': f'{schema_type} 仅支持版本 {expected_version}'})
        if schema_type == ApplicationBaselineCheck.SchemaType.JSON_SCHEMA:
            self._validate_json_schema(schema_content)
        elif schema_type == ApplicationBaselineCheck.SchemaType.REGEXP:
            self._validate_regexp(schema_content)
        else:
            self._validate_xml_schema(schema_content, schema_type)
        return attrs

    @staticmethod
    def _validate_json_schema(content):
        try:
            schema = json.loads(content)
            Draft202012Validator.check_schema(schema)
        except (json.JSONDecodeError, SchemaError) as exc:
            raise serializers.ValidationError(f'JSON Schema 无效: {exc}') from exc

    @staticmethod
    def _validate_regexp(content):
        try:
            rule = json.loads(content)
        except json.JSONDecodeError as exc:
            raise serializers.ValidationError(f'Regexp 规则必须是有效 JSON: {exc}') from exc
        if not isinstance(rule, dict) or not isinstance(rule.get('pattern'), str) or not rule['pattern']:
            raise serializers.ValidationError('Regexp 规则必须包含非空 pattern')
        if rule.get('expect') not in {'present', 'absent'}:
            raise serializers.ValidationError('Regexp expect 仅支持 present 或 absent')
        unknown_fields = set(rule) - {'pattern', 'expect'}
        if unknown_fields:
            raise serializers.ValidationError(f'Regexp 规则包含未知字段: {", ".join(sorted(unknown_fields))}')

    @staticmethod
    def _validate_xml_schema(content, schema_type):
        try:
            root = ET.fromstring(content)
        except ET.ParseError as exc:
            raise serializers.ValidationError(f'{schema_type} 文档无效: {exc}') from exc
        expected_root = '{http://purl.oclc.org/dsdl/schematron}schema'
        if root.tag != expected_root:
            raise serializers.ValidationError(f'{schema_type} 根元素或命名空间无效')
        assertions = root.findall('.//{http://purl.oclc.org/dsdl/schematron}assert')
        if not assertions or any(not str(assertion.get('test') or '').strip() for assertion in assertions):
            raise serializers.ValidationError('Schematron 必须至少包含一个有效 assert')


class ApplicationSerializer(ModelSerializer):
    versions = ApplicationVersionSerializer(many=True, read_only=True)
    version_count = serializers.IntegerField(read_only=True)
    deployment_template_count = serializers.IntegerField(read_only=True)
    deployment_count = serializers.IntegerField(read_only=True)
    baseline_checks = ApplicationBaselineCheckSerializer(many=True, required=False)

    class Meta:
        model = Application
        fields = '__all__'

    def validate_baseline_checks(self, checks):
        names = [str(check.get('name') or '').strip() for check in checks]
        if len(names) != len(set(names)):
            raise serializers.ValidationError('同一应用的基线检查项名称不能重复')
        return checks

    @staticmethod
    def _save_baseline_checks(instance, checks):
        if checks is None:
            return
        ApplicationBaselineCheck.objects.filter(application=instance).delete()
        ApplicationBaselineCheck.objects.bulk_create([
            ApplicationBaselineCheck(application=instance, **check) for check in checks
        ])

    @transaction.atomic
    def create(self, validated_data):
        baseline_checks = validated_data.pop('baseline_checks', [])
        instance = Application.objects.create(**validated_data)
        self._save_baseline_checks(instance, baseline_checks)
        return instance

    @transaction.atomic
    def update(self, instance, validated_data):
        baseline_checks = validated_data.pop('baseline_checks', None)
        instance = super().update(instance, validated_data)
        self._save_baseline_checks(instance, baseline_checks)
        return instance


class ApplicationDeploymentTemplateSerializer(ModelSerializer):
    application_name = serializers.CharField(source='application.name', read_only=True)
    ports = ApplicationPortSerializer(many=True, required=False)
    paths = ApplicationPathSerializer(many=True, required=False)
    config_files = ApplicationConfigFileSerializer(many=True, required=False)
    logs = ApplicationLogDefinitionSerializer(many=True, required=False)
    control_actions = ApplicationControlActionSerializer(many=True, required=False)
    docker_config = DockerControlConfigSerializer(required=False, allow_null=True)
    compose_config = DockerComposeControlConfigSerializer(required=False, allow_null=True)

    class Meta:
        model = ApplicationDeploymentTemplate
        fields = '__all__'

    def validate(self, attrs):
        control_type = attrs.get('control_type', getattr(self.instance, 'control_type', None))
        service_name = attrs.get('service_name', getattr(self.instance, 'service_name', ''))
        app_home = attrs.get('app_home', getattr(self.instance, 'app_home', ''))
        run_user = attrs.get('run_user', getattr(self.instance, 'run_user', ''))
        actions = attrs.get('control_actions')
        docker_config = attrs.get('docker_config')
        compose_config = attrs.get('compose_config')

        if control_type == ApplicationDeploymentTemplate.ControlType.SYSTEMD and not str(service_name or '').strip():
            raise serializers.ValidationError({'service_name': 'Systemd 模板必须填写服务名'})
        if control_type == ApplicationDeploymentTemplate.ControlType.COMMAND:
            submitted_actions = {item.get('action') for item in (actions or [])}
            if actions is not None and not {'start', 'stop', 'status'}.issubset(submitted_actions):
                raise serializers.ValidationError({'control_actions': '命令行模板必须配置 start、stop、status'})
            if self.instance is None and actions is None:
                raise serializers.ValidationError({'control_actions': '命令行模板必须配置 start、stop、status'})
        if control_type == ApplicationDeploymentTemplate.ControlType.DOCKER and docker_config is None:
            if self.instance is None or not hasattr(self.instance, 'docker_config'):
                raise serializers.ValidationError({'docker_config': 'Docker 模板必须配置容器名称'})
        if control_type == ApplicationDeploymentTemplate.ControlType.DOCKER_COMPOSE and compose_config is None:
            if self.instance is None or not hasattr(self.instance, 'compose_config'):
                raise serializers.ValidationError({'compose_config': 'Docker Compose 模板必须配置项目和服务'})
        if control_type == ApplicationDeploymentTemplate.ControlType.EXTERNAL_HA:
            resource_name = attrs.get('ha_resource_name', getattr(self.instance, 'ha_resource_name', ''))
            if not str(resource_name or '').strip():
                raise serializers.ValidationError({'ha_resource_name': '外部 HA 模板必须填写资源名称'})
        variable_values = [app_home, attrs.get('work_directory', '')]
        variable_values.extend(item.get('path', '') for item in (attrs.get('paths') or []))
        variable_values.extend(item.get('path', '') for item in (attrs.get('config_files') or []))
        variable_values.extend(item.get('path_pattern', '') for item in (attrs.get('logs') or []))
        variable_values.extend(item.get('command', '') for item in (actions or []))
        if compose_config:
            variable_values.extend([
                compose_config.get('compose_file_path', ''),
                compose_config.get('working_directory', ''),
                compose_config.get('env_file', ''),
            ])
        try:
            if APP_HOME_VARIABLE in str(app_home or ''):
                raise ApplicationVariableError('App Home 不能引用自身')
            if APP_HOME_VARIABLE in str(run_user or '') or RUN_USER_VARIABLE in str(run_user or ''):
                raise ApplicationVariableError('运行用户不能引用模板变量')
            for value in variable_values:
                resolve_application_variables(value, app_home=app_home, run_user=run_user)
        except ApplicationVariableError as exc:
            raise serializers.ValidationError({'variables': str(exc)}) from exc
        return attrs

    @staticmethod
    def _pop_nested(validated_data):
        nested_fields = (
            'ports', 'paths', 'config_files', 'logs', 'control_actions',
            'docker_config', 'compose_config',
        )
        return {field: validated_data.pop(field) for field in nested_fields if field in validated_data}

    @staticmethod
    def _save_nested(instance, nested_data):
        many_relations = {
            'ports': ApplicationPort,
            'paths': ApplicationPath,
            'config_files': ApplicationConfigFile,
            'logs': ApplicationLogDefinition,
            'control_actions': ApplicationControlAction,
        }
        for field_name, model in many_relations.items():
            if field_name not in nested_data:
                continue
            model.objects.filter(deployment_template=instance).delete()
            model.objects.bulk_create([
                model(deployment_template=instance, **item) for item in nested_data[field_name]
            ])

        one_relations = {
            'docker_config': DockerControlConfig,
            'compose_config': DockerComposeControlConfig,
        }
        for field_name, model in one_relations.items():
            if field_name not in nested_data:
                continue
            model.objects.filter(deployment_template=instance).delete()
            item = nested_data[field_name]
            if item is not None:
                model.objects.create(deployment_template=instance, **item)

    @transaction.atomic
    def create(self, validated_data):
        nested_data = self._pop_nested(validated_data)
        instance = ApplicationDeploymentTemplate.objects.create(**validated_data)
        self._save_nested(instance, nested_data)
        return instance

    @transaction.atomic
    def update(self, instance, validated_data):
        nested_data = self._pop_nested(validated_data)
        instance = super().update(instance, validated_data)
        self._save_nested(instance, nested_data)
        return instance


class ApplicationBaselineResultSerializer(ModelSerializer):
    class Meta:
        model = ApplicationBaselineResult
        fields = '__all__'


class ApplicationBaselineExecutionSerializer(ModelSerializer):
    results = ApplicationBaselineResultSerializer(many=True, read_only=True)
    deployment_name = serializers.CharField(source='deployment.instance_name', read_only=True)
    application_name = serializers.CharField(source='deployment.application_version.application.name', read_only=True)
    host_name = serializers.CharField(source='deployment.host.instance_name', read_only=True)
    host_ip = serializers.IPAddressField(source='deployment.host.ip', read_only=True)
    job_id = serializers.CharField(source='agent_job.job_id', read_only=True)

    class Meta:
        model = ApplicationBaselineExecution
        fields = '__all__'


class ApplicationDeploymentSerializer(ModelSerializer):
    application_id = serializers.IntegerField(source='application_version.application_id', read_only=True)
    application_name = serializers.CharField(source='application_version.application.name', read_only=True)
    version = serializers.CharField(source='application_version.version', read_only=True)
    host_name = serializers.CharField(source='host.instance_name', read_only=True)
    host_ip = serializers.IPAddressField(source='host.ip', read_only=True)
    template_name = serializers.CharField(source='deployment_template.name', read_only=True)
    control_type = serializers.CharField(source='deployment_template.control_type', read_only=True)
    run_user = serializers.CharField(source='deployment_template.run_user', read_only=True)
    run_group = serializers.CharField(source='deployment_template.run_group', read_only=True)
    app_home = serializers.CharField(source='deployment_template.app_home', read_only=True)
    work_directory = serializers.CharField(source='deployment_template.work_directory', read_only=True)
    ports = ApplicationPortSerializer(source='deployment_template.ports', many=True, read_only=True)

    class Meta:
        model = ApplicationDeployment
        fields = '__all__'

    def validate(self, attrs):
        application_version = attrs.get('application_version', getattr(self.instance, 'application_version', None))
        deployment_template = attrs.get('deployment_template', getattr(self.instance, 'deployment_template', None))
        if application_version and deployment_template and application_version.application_id != deployment_template.application_id:
            raise serializers.ValidationError({'deployment_template': '部署模板与应用版本必须属于同一个应用'})
        return attrs


class HostGroupSerializer(ModelSerializer):
    parent_id = serializers.IntegerField(required=False, allow_null=True)
    parent_name = serializers.SerializerMethodField()
    host_count = serializers.SerializerMethodField()

    class Meta:
        model = HostGroup
        fields = '__all__'

    @staticmethod
    def _get_max_tree_depth():
        """从 sys_config 读取主机分组最大层级，首次调用自动写入默认值。"""
        from sys_config.models import SysConfig
        config, _ = SysConfig.objects.get_or_create(
            key='sys.assets.hostgroup.max_tree_depth',
            defaults={
                'value': '5',
                'default_value': '5',
                'value_type': 'int',
                'name': '主机分组最大层级',
                'description': '主机分组树形结构的最大嵌套层数，修改后重启生效',
                'is_readonly': False,
            },
        )
        try:
            return max(1, int(str(config.value).strip()))
        except (ValueError, TypeError):
            return 5

    def get_parent_name(self, obj):
        if obj.parent:
            return obj.parent.name
        return ''

    def get_host_count(self, obj):
        # 递归统计该分组及所有子分组的主机总数
        def count_hosts_recursive(group):
            count = group.host_set.count()
            for child in group.children.all():
                count += count_hosts_recursive(child)
            return count
        return count_hosts_recursive(obj)

    def _get_group_depth(self, group):
        depth = 1
        parent = group.parent
        while parent is not None:
            depth += 1
            parent = parent.parent
        return depth

    def _validate_parent_depth(self, parent_id):
        if parent_id in (0, "0", "", None):
            return None
        parent = HostGroup.objects.filter(id=parent_id).select_related('parent').first()
        if not parent:
            raise serializers.ValidationError({"parent_id": ["上级分组不存在"]})
        max_depth = self._get_max_tree_depth()
        if self._get_group_depth(parent) >= max_depth:
            raise serializers.ValidationError({"parent_id": [f"分组层级不能超过{max_depth}层"]})
        return parent_id

    def create(self, validated_data):
        parent_id = validated_data.pop("parent_id", None)
        parent_id = self._validate_parent_depth(parent_id)
        validated_data["parent_id"] = parent_id
        validated_data["create_time"] = timezone.now()
        data = HostGroup.objects.create(**validated_data)
        return data

    def update(self, instance, validated_data):
        parent_id = validated_data.pop("parent_id", instance.parent_id)
        parent_id = self._validate_parent_depth(parent_id)
        instance.parent_id = parent_id
        instance.name = validated_data.get("name", instance.name)
        instance.remark = validated_data.get("remark", instance.remark)
        instance.update_time = timezone.now()
        instance.save()
        return instance

# host 
class HostSerializer(ModelSerializer):
    group_id = serializers.IntegerField(required=False, allow_null=True, write_only=True)
    group_name = serializers.SerializerMethodField()
    system = serializers.SerializerMethodField()
    hardware = serializers.SerializerMethodField()
    runtime = serializers.SerializerMethodField()
    disks = serializers.SerializerMethodField()
    # 系统信息顶级字段
    os_type = serializers.SerializerMethodField()
    os_version = serializers.SerializerMethodField()
    kernel_version = serializers.SerializerMethodField()
    hostname = serializers.SerializerMethodField()
    cpu_cores = serializers.SerializerMethodField()
    cpu_model = serializers.SerializerMethodField()
    memory_gb = serializers.SerializerMethodField()
    disk_total_gb = serializers.SerializerMethodField()
    disk_used_percent = serializers.SerializerMethodField()
    last_collect_time = serializers.SerializerMethodField()
    architecture = serializers.SerializerMethodField()
    monitors = serializers.SerializerMethodField()
    create_time = serializers.SerializerMethodField()
    update_time = serializers.SerializerMethodField()

    class Meta:
        model = Host
        fields = '__all__'

    @staticmethod
    def _normalize_webssh_user_list(raw_value):
        username_regexp = re.compile(r'^[a-zA-Z_][a-zA-Z0-9._-]{0,63}$')
        tokens = [
            token.strip()
            for token in str(raw_value or '').split()
            if token.strip() and username_regexp.match(token.strip())
        ]
        unique_tokens = list(dict.fromkeys(tokens))
        return unique_tokens or ['root']

    def _normalize_webssh_preferences(self, data, instance=None):
        if instance is not None and 'webssh_login_users' not in data and 'webssh_default_username' not in data:
            return data

        raw_users = data.get('webssh_login_users')
        if raw_users is None and instance is not None:
            raw_users = instance.webssh_login_users

        raw_default = data.get('webssh_default_username')
        if raw_default is None and instance is not None:
            raw_default = instance.webssh_default_username

        users = self._normalize_webssh_user_list(raw_users)
        default_user = str(raw_default or '').strip()
        if default_user not in users:
            default_user = users[0]
        data['webssh_login_users'] = ' '.join(users)
        data['webssh_default_username'] = default_user
        return data

    def get_group_name(self, obj):
        return obj.group.name if obj.group else ''

    def get_system(self, obj):
        system = getattr(obj, 'system', None)
        agent_version = getattr(system, 'agent_version', None)
        # 兼容历史脏值：ssh-collector 仅表示采集方式，不应作为 agent 版本展示。
        if str(agent_version or '').strip().lower() == 'ssh-collector':
            agent_version = None

        # Agent 状态属于 Host，不依赖 system 快照；没有快照时也必须保留在线状态。
        agent_last_seen_at = getattr(obj, 'agent_online_time', None)
        agent_online = bool(getattr(obj, 'agent_online', False))

        return {
            'os_type': getattr(system, 'os_type', None),
            'os_version': getattr(system, 'os_version', None),
            'kernel_version': getattr(system, 'kernel_version', None),
            'hostname': getattr(system, 'hostname', None),
            'agent_version': agent_version,
            'timezone_name': getattr(system, 'timezone_name', None),
            'utc_offset': getattr(system, 'utc_offset', None),
            'collector_source': getattr(system, 'collector_source', None),
            'agent_last_seen_at': agent_last_seen_at,
            'agent_online': agent_online,
        }

    def get_hardware(self, obj):
        hardware = getattr(obj, 'hardware', None)
        if not hardware:
            return None
        return {
            'cpu_cores': hardware.cpu_cores,
            'cpu_model': hardware.cpu_model,
            'memory_gb': hardware.memory_gb,
            'disk_total_gb': hardware.disk_total_gb,
            'disk_used_percent': self._calc_disk_usage_percent(obj),
            'architecture': hardware.architecture,
        }

    def get_runtime(self, obj):
        runtime = getattr(obj, 'runtime', None)
        if runtime is None:
            return None
        return {
            'cpu_usage_percent': runtime.cpu_usage_percent,
            'cpu_times': runtime.cpu_times,
            'memory_usage_percent': runtime.memory_usage_percent,
            'memory': runtime.memory,
            'disk_io': runtime.disk_io,
            'os_uptime_seconds': runtime.os_uptime_seconds,
            'os_boot_time': runtime.os_boot_time,
            'metrics_sample_window_ms': runtime.metrics_sample_window_ms,
            'collected_at': runtime.collected_at,
        }

    def _calc_disk_usage_percent(self, obj):
        total_size = 0.0
        total_used = 0.0
        for disk in obj.disks.all():
            if _should_ignore_disk(disk.device, disk.filesystem):
                continue
            if disk.size_gb is None or disk.used_gb is None:
                continue
            if disk.size_gb <= 0:
                continue
            total_size += float(disk.size_gb)
            total_used += float(disk.used_gb)

        if total_size <= 0:
            return None

        return round((total_used / total_size) * 100, 2)

    def get_disks(self, obj):
        return [
            {
                'device': disk.device,
                'mount_point': disk.mount_point,
                'size_gb': disk.size_gb,
                'used_gb': disk.used_gb,
                'filesystem': disk.filesystem,
                'usage_percent': round((float(disk.used_gb) / float(disk.size_gb)) * 100, 2)
                if (disk.size_gb not in (None, 0) and disk.used_gb is not None)
                else None,
            }
            for disk in obj.disks.all()
            if not _should_ignore_disk(disk.device, disk.filesystem)
        ]

    # 系统信息顶级字段的 getter 方法
    def get_os_type(self, obj):
        system = getattr(obj, 'system', None)
        return system.os_type if system else None

    def get_os_version(self, obj):
        system = getattr(obj, 'system', None)
        return system.os_version if system else None

    def get_kernel_version(self, obj):
        system = getattr(obj, 'system', None)
        return system.kernel_version if system else None

    def get_hostname(self, obj):
        system = getattr(obj, 'system', None)
        return system.hostname if system else None

    def get_cpu_cores(self, obj):
        hardware = getattr(obj, 'hardware', None)
        return hardware.cpu_cores if hardware else None

    def get_cpu_model(self, obj):
        hardware = getattr(obj, 'hardware', None)
        return hardware.cpu_model if hardware else None

    def get_memory_gb(self, obj):
        hardware = getattr(obj, 'hardware', None)
        return hardware.memory_gb if hardware else None

    def get_disk_total_gb(self, obj):
        hardware = getattr(obj, 'hardware', None)
        return hardware.disk_total_gb if hardware else None

    def get_disk_used_percent(self, obj):
        return self._calc_disk_usage_percent(obj)

    def get_architecture(self, obj):
        hardware = getattr(obj, 'hardware', None)
        return hardware.architecture if hardware else None

    def get_last_collect_time(self, obj):
        # 统一以 Host.collect_time 为准，避免与在线状态/子表时间戳出现口径分叉。
        return getattr(obj, 'collect_time', None)

    def get_monitors(self, obj):
        # 一台主机可能纳管多个监控项（exporter_type 各不相同），供主机编辑表单展示/编辑监控设置；
        # 脚本内容等详细字段请通过 /monitor/targets/{id}/ 单独获取，避免主机详情负载过大。
        targets = obj.monitor_targets.all()
        return [
            {
                'id': target.id,
                'name': target.exporter_type,
                'port': int(getattr(target, 'scrape_port', 9100) or 9100),
                'enabled': bool(target.managed_enabled),
                'install_status': str(target.install_status or 'unknown'),
                'install_message': target.install_message,
                'retry_count': target.retry_count,
                'update_time': self._serialize_datetime_like(target.update_time),
            }
            for target in targets
        ]

    def _serialize_datetime_like(self, value):
        if value is None:
            return None
        if isinstance(value, datetime):
            return value
        if hasattr(value, 'year') and hasattr(value, 'month') and hasattr(value, 'day'):
            return datetime(value.year, value.month, value.day)
        return value

    def get_create_time(self, obj):
        return self._serialize_datetime_like(getattr(obj, 'create_time', None))

    def get_update_time(self, obj):
        return self._serialize_datetime_like(getattr(obj, 'update_time', None))
    
    # 创建
    def create(self, validated_data):
        validated_data = self._normalize_webssh_preferences(validated_data)
        group_id = validated_data.pop('group_id', None)
        if group_id in (0, '0', '', None):
            group_id = None
        validated_data['group_id'] = group_id
        validated_data["create_time"] = timezone.now()
        return Host.objects.create(**validated_data)

    def update(self, instance, validated_data):
        validated_data = self._normalize_webssh_preferences(validated_data, instance=instance)
        group_id = validated_data.pop('group_id', serializers.empty)

        if group_id is not serializers.empty:
            if group_id in (0, '0', '', None):
                instance.group_id = None
            else:
                instance.group_id = group_id

        for attr, value in validated_data.items():
            setattr(instance, attr, value)

        instance.update_time = timezone.now()
        instance.save()
        return instance


class HostListSerializer(ModelSerializer):
    group_name = serializers.SerializerMethodField()
    system = serializers.SerializerMethodField()
    hardware = serializers.SerializerMethodField()
    os_type = serializers.SerializerMethodField()
    os_version = serializers.SerializerMethodField()
    kernel_version = serializers.SerializerMethodField()
    monitor_enabled = serializers.SerializerMethodField()
    monitor_install_status = serializers.SerializerMethodField()

    class Meta:
        model = Host
        fields = [
            'id',
            'instance_name',
            'agent_id',
            'ip',
            'remark',
            'webssh_default_username',
            'webssh_login_users',
            'agent_online',
            'collect_status',
            'group_name',
            'system',
            'hardware',
            'os_type',
            'os_version',
            'kernel_version',
            'monitor_enabled',
            'monitor_install_status',
        ]

    def get_group_name(self, obj):
        return obj.group.name if obj.group else ''

    def get_system(self, obj):
        system = getattr(obj, 'system', None)
        agent_version = getattr(system, 'agent_version', None)
        if str(agent_version or '').strip().lower() == 'ssh-collector':
            agent_version = None

        # Agent 状态属于 Host，不能因为没有 system 快照而丢失。
        agent_last_seen_at = getattr(obj, 'agent_online_time', None)
        agent_online = bool(getattr(obj, 'agent_online', False))

        return {
            'hostname': getattr(system, 'hostname', None),
            'agent_version': agent_version,
            'agent_last_seen_at': agent_last_seen_at,
            'agent_online': agent_online,
        }

    def get_hardware(self, obj):
        hardware = getattr(obj, 'hardware', None)
        if not hardware:
            return None
        return {
            'cpu_cores': hardware.cpu_cores,
            'cpu_model': hardware.cpu_model,
            'memory_gb': hardware.memory_gb,
            'disk_total_gb': hardware.disk_total_gb,
            'disk_used_percent': self._calc_disk_usage_percent(obj),
            'architecture': hardware.architecture,
        }

    def _calc_disk_usage_percent(self, obj):
        total_size = 0.0
        total_used = 0.0
        for disk in obj.disks.all():
            if _should_ignore_disk(disk.device, disk.filesystem):
                continue
            if disk.size_gb is None or disk.used_gb is None:
                continue
            if disk.size_gb <= 0:
                continue
            total_size += float(disk.size_gb)
            total_used += float(disk.used_gb)

        if total_size <= 0:
            return None

        return round((total_used / total_size) * 100, 2)

    def get_os_type(self, obj):
        system = getattr(obj, 'system', None)
        return system.os_type if system else None

    def get_os_version(self, obj):
        system = getattr(obj, 'system', None)
        return system.os_version if system else None

    def get_kernel_version(self, obj):
        system = getattr(obj, 'system', None)
        return system.kernel_version if system else None

    def _get_monitor_target(self, obj):
        cached_targets = getattr(obj, 'monitor_targets', None)
        if cached_targets is not None:
            for item in cached_targets.all():
                if str(getattr(item, 'exporter_type', '') or '') == 'node_exporter':
                    return item
        return obj.monitor_targets.filter(exporter_type='node_exporter').order_by('-id').first()

    def get_monitor_enabled(self, obj):
        target = self._get_monitor_target(obj)
        return bool(target and target.managed_enabled)

    def get_monitor_install_status(self, obj):
        target = self._get_monitor_target(obj)
        if not target:
            return 'unknown'
        return str(target.install_status or 'unknown')


class WebSSHSessionLogSerializer(ModelSerializer):
    host_name = serializers.SerializerMethodField()
    host_ip = serializers.SerializerMethodField()

    class Meta:
        model = WebSSHSessionLog
        fields = [
            'id',
            'host',
            'host_name',
            'host_ip',
            'user_id',
            'username',
            'requested_username',
            'effective_username',
            'switch_user_status',
            'switch_user_error',
            'client_ip',
            'user_agent',
            'status',
            'start_time',
            'end_time',
            'duration_seconds',
            'close_code',
            'error_message',
            'input_bytes',
            'command_count',
            'recorded_content_bytes',
            'is_content_truncated',
        ]

    def get_host_name(self, obj):
        system = getattr(obj.host, 'system', None)
        hostname = getattr(system, 'hostname', None) if system else None
        return obj.host.instance_name or hostname or f'Host-{obj.host.id}'

    def get_host_ip(self, obj):
        return obj.host.ip


def _should_ignore_disk(device, filesystem=None):
    if bool(re.match(r'^/dev/sr\d+$', (device or '').strip())):
        return True
    return str(filesystem or '').strip().lower() == 'squashfs'
