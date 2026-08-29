from rest_framework.serializers import ModelSerializer
from datetime import datetime, timedelta
import re
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


class ApplicationSerializer(ModelSerializer):
    versions = ApplicationVersionSerializer(many=True, read_only=True)
    version_count = serializers.IntegerField(read_only=True)
    deployment_template_count = serializers.IntegerField(read_only=True)
    deployment_count = serializers.IntegerField(read_only=True)

    class Meta:
        model = Application
        fields = '__all__'


class BusinessSystemSerializer(ModelSerializer):
    deployment_count = serializers.IntegerField(read_only=True)
    environment_count = serializers.IntegerField(read_only=True)
    project_name = serializers.CharField(source='project.name', read_only=True, default='')
    project_code = serializers.CharField(source='project.code', read_only=True, default='')

    class Meta:
        model = BusinessSystem
        fields = '__all__'
        extra_kwargs = {
            # 项目编码要拼进日志索引名，缺失会直接导致 fluent-bit 配置生成失败，故在 API 层就拦住。
            'project': {'required': True, 'allow_null': False},
        }


class ProjectSerializer(ModelSerializer):
    # 归属关系的唯一写入口是 BusinessSystem.project；这里只读，避免从项目侧移除时把业务系统的项目置空。
    business_systems = serializers.PrimaryKeyRelatedField(many=True, read_only=True)
    business_system_names = serializers.SerializerMethodField()

    class Meta:
        model = Project
        fields = '__all__'

    def get_business_system_names(self, obj):
        return list(obj.business_systems.values_list('name', flat=True))


class BusinessEnvironmentSerializer(ModelSerializer):
    service_count = serializers.IntegerField(read_only=True)
    deployment_count = serializers.IntegerField(read_only=True)

    class Meta:
        model = BusinessEnvironment
        fields = '__all__'


class ClusterProfileSerializer(ModelSerializer):
    application_name = serializers.CharField(source='application.name', read_only=True)
    service_count = serializers.IntegerField(read_only=True)

    class Meta:
        model = ClusterProfile
        fields = '__all__'

    def validate(self, attrs):
        profile_type = attrs.get('profile_type', getattr(self.instance, 'profile_type', ClusterProfile.ProfileType.CUSTOM))
        cluster_type = attrs.get('cluster_type', getattr(self.instance, 'cluster_type', ClusterProfile.ClusterType.CUSTOM))
        if self.instance is not None and self.instance.profile_type == ClusterProfile.ProfileType.BUILTIN:
            raise serializers.ValidationError('内置集群模型由系统维护，不能修改')
        if profile_type != ClusterProfile.ProfileType.CUSTOM or cluster_type != ClusterProfile.ClusterType.CUSTOM:
            raise serializers.ValidationError('只能新增或修改自定义集群模型')
        if attrs.get('application', getattr(self.instance, 'application', None)) is None:
            raise serializers.ValidationError({'application': '自定义集群模型必须选择应用'})
        attrs['profile_type'] = ClusterProfile.ProfileType.CUSTOM
        attrs['cluster_type'] = ClusterProfile.ClusterType.CUSTOM
        return attrs


class ApplicationServiceSerializer(ModelSerializer):
    business_system_name = serializers.CharField(source='business_system.name', read_only=True)
    environment_name = serializers.CharField(source='environment.name', read_only=True)
    environment_code = serializers.CharField(source='environment.code', read_only=True)
    application_name = serializers.CharField(source='application.name', read_only=True)
    application_version_name = serializers.CharField(source='application_version.version', read_only=True)
    deployment_template_name = serializers.CharField(source='deployment_template.name', read_only=True)
    cluster_profile_name = serializers.CharField(source='cluster_profile.name', read_only=True)
    cluster_type = serializers.CharField(source='cluster_profile.cluster_type', read_only=True)
    deployment_count = serializers.IntegerField(read_only=True)
    ports = ApplicationPortSerializer(source='deployment_template.ports', many=True, read_only=True)
    member_configs = serializers.ListField(
        child=serializers.DictField(), write_only=True, required=False,
    )
    member_instances = serializers.SerializerMethodField()

    class Meta:
        model = ApplicationService
        fields = '__all__'
        extra_kwargs = {'application': {'required': False}}

    def validate(self, attrs):
        environment = attrs.get('environment', getattr(self.instance, 'environment', None))
        if environment is None:
            raise serializers.ValidationError({'environment': '请选择所属环境'})
        topology_type = attrs.get('topology_type', getattr(self.instance, 'topology_type', None))
        application = attrs.get('application', getattr(self.instance, 'application', None))
        cluster_profile = attrs.get('cluster_profile', getattr(self.instance, 'cluster_profile', None))
        application_version = attrs.get('application_version', getattr(self.instance, 'application_version', None))
        deployment_template = attrs.get('deployment_template', getattr(self.instance, 'deployment_template', None))
        macro_values = attrs.get(
            'macro_values',
            {} if 'deployment_template' in attrs else getattr(self.instance, 'macro_values', {}),
        )
        if not isinstance(macro_values, dict):
            raise serializers.ValidationError({'macro_values': '宏值必须是 JSON 对象'})
        macro_names = {
            item['name'] for item in (deployment_template.macro_definitions or [])
            if isinstance(item, dict) and item.get('name')
        } if deployment_template else set()
        unknown_macros = set(macro_values) - macro_names
        if unknown_macros:
            raise serializers.ValidationError({'macro_values': f'包含未定义宏: {", ".join(sorted(unknown_macros))}'})
        if any('\n' in str(value) or '\r' in str(value) for value in macro_values.values()):
            raise serializers.ValidationError({'macro_values': '宏值不能包含换行符'})
        access_address = attrs.get('access_address', getattr(self.instance, 'access_address', ''))
        if topology_type == ApplicationService.TopologyType.STANDALONE:
            attrs['access_address'] = ''
            access_address = ''
        if topology_type == ApplicationService.TopologyType.CLUSTER and cluster_profile is None:
            raise serializers.ValidationError({'cluster_profile': '集群服务必须选择集群模型'})
        if topology_type != ApplicationService.TopologyType.CLUSTER and cluster_profile is not None:
            raise serializers.ValidationError({'cluster_profile': '非集群服务不能选择集群模型'})
        if topology_type == ApplicationService.TopologyType.LOAD_BALANCER and not str(access_address or '').strip():
            raise serializers.ValidationError({'access_address': '负载均衡形态必须填写入口地址'})
        if (
            topology_type == ApplicationService.TopologyType.CLUSTER
            and cluster_profile is not None
            and cluster_profile.application is not None
        ):
            application = cluster_profile.application
            attrs['application'] = application
        elif application is None:
            raise serializers.ValidationError({'application': '请选择应用'})
        if application_version and application_version.application_id != application.id:
            raise serializers.ValidationError({'application_version': '应用版本必须属于当前应用'})
        if deployment_template and deployment_template.application_id != application.id:
            raise serializers.ValidationError({'deployment_template': '部署模板必须属于当前应用'})
        if (
            cluster_profile
            and cluster_profile.cluster_type == ClusterProfile.ClusterType.HA
            and deployment_template
            and deployment_template.control_type != ApplicationDeploymentTemplate.ControlType.EXTERNAL_HA
        ):
            raise serializers.ValidationError({'deployment_template': 'HA 集群必须使用外部 HA 部署模板'})
        if (
            deployment_template
            and deployment_template.control_type == ApplicationDeploymentTemplate.ControlType.EXTERNAL_HA
            and not (cluster_profile and cluster_profile.cluster_type == ClusterProfile.ClusterType.HA)
        ):
            raise serializers.ValidationError({'deployment_template': '只有 HA 集群可以使用外部 HA 部署模板'})
        if application_version and deployment_template and application_version.application_id != deployment_template.application_id:
            raise serializers.ValidationError({'deployment_template': '部署模板与应用版本必须属于同一个应用'})
        member_configs = attrs.get('member_configs')
        uses_members = topology_type in (
            ApplicationService.TopologyType.STANDALONE,
            ApplicationService.TopologyType.CLUSTER,
            ApplicationService.TopologyType.LOAD_BALANCER,
        )
        allow_empty_draft = bool(self.context.get('request') and self.context['request'].data.get('draft'))
        if uses_members and member_configs is None and self.instance is None and not allow_empty_draft:
            raise serializers.ValidationError({'member_configs': '集群或负载均衡服务必须选择至少一个部署实例'})
        if uses_members and member_configs is not None and (member_configs or not allow_empty_draft):
            if not member_configs:
                raise serializers.ValidationError({'member_configs': '集群或负载均衡服务必须选择至少一个部署实例'})
            deployment_ids = [item.get('deployment') for item in member_configs]
            if any(not deployment_id for deployment_id in deployment_ids) or len(deployment_ids) != len(set(deployment_ids)):
                raise serializers.ValidationError({'member_configs': '集群成员实例不能为空或重复'})
            is_ha_cluster = cluster_profile and cluster_profile.cluster_type == ClusterProfile.ClusterType.HA
            if is_ha_cluster and len(deployment_ids) < 2:
                raise serializers.ValidationError({'member_configs': 'HA 集群至少需要两个成员实例'})
            deployments = ApplicationDeployment.objects.filter(id__in=deployment_ids)
            if deployments.count() != len(deployment_ids):
                raise serializers.ValidationError({'member_configs': '包含不存在的部署实例'})
            # 实例所属应用现在由其逻辑服务决定，跨应用复用实例会造成配置冲突。
            # 当前服务的应用本次就要改成 application，不能用它的旧值判定冲突。
            conflicting_services = ApplicationService.objects.filter(
                deployment_links__deployment_id__in=deployment_ids,
            ).exclude(application=application)
            if self.instance is not None:
                conflicting_services = conflicting_services.exclude(pk=self.instance.pk)
            if conflicting_services.exists():
                raise serializers.ValidationError({'member_configs': '集群成员实例必须属于同一应用'})
        if cluster_profile and cluster_profile.cluster_type == ClusterProfile.ClusterType.HA:
            if not str(access_address or '').strip():
                raise serializers.ValidationError({'access_address': 'HA 集群必须填写 VIP'})
        return attrs

    def get_member_instances(self, obj):
        is_ha = obj.cluster_profile and obj.cluster_profile.cluster_type == ClusterProfile.ClusterType.HA
        vip = str(obj.access_address or '').strip()
        return [
            {
                'deployment': link.deployment_id,
                'instance_name': link.deployment.instance_name,
                'host_name': link.deployment.host.instance_name,
                'host_ip': link.deployment.host.ip,
                'enabled': link.enabled,
                'ha_role': (
                    ApplicationDeployment.HaRole.PRIMARY
                    if is_ha and vip and str(link.deployment.host.ip or '').strip() == vip
                    else ApplicationDeployment.HaRole.STANDBY
                    if is_ha and vip
                    else ApplicationDeployment.HaRole.UNKNOWN
                ),
            }
            for link in obj.deployment_links.select_related('deployment__host').order_by('deployment__instance_name', 'deployment_id')
        ]

    @staticmethod
    def _save_members(instance, member_configs):
        if member_configs is None:
            return
        deployment_ids = [item['deployment'] for item in member_configs]
        ApplicationServiceDeployment.objects.filter(service=instance).exclude(deployment_id__in=deployment_ids).delete()
        for item in member_configs:
            ApplicationServiceDeployment.objects.update_or_create(
                service=instance,
                deployment_id=item['deployment'],
                defaults={
                    'enabled': item.get('enabled', True),
                },
            )

    @transaction.atomic
    def create(self, validated_data):
        member_configs = validated_data.pop('member_configs', None)
        instance = super().create(validated_data)
        self._save_members(instance, member_configs)
        return instance

    @transaction.atomic
    def update(self, instance, validated_data):
        member_configs = validated_data.pop('member_configs', None)
        instance = super().update(instance, validated_data)
        self._save_members(instance, member_configs)
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
        macro_definitions = attrs.get('macro_definitions', getattr(self.instance, 'macro_definitions', []))
        if not isinstance(macro_definitions, list) or any(
            not isinstance(item, dict)
            or not re.fullmatch(r'[A-Za-z_][A-Za-z0-9_]*', str(item.get('name') or ''))
            or any(key not in {'name', 'value', 'description'} for key in item)
            or '\n' in str(item.get('value') or '')
            or '\r' in str(item.get('value') or '')
            for item in macro_definitions
        ):
            raise serializers.ValidationError({'macro_definitions': '宏定义必须包含合法的 Key、Value、说明'})
        macro_names = [item['name'] for item in macro_definitions]
        if len(macro_names) != len(set(macro_names)):
            raise serializers.ValidationError({'macro_definitions': '宏名称不能重复'})
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
            submitted_actions = {item.get('action') for item in (actions or [])}
            if actions is not None and 'status' not in submitted_actions:
                raise serializers.ValidationError({'control_actions': '外部 HA 模板必须配置状态检查命令'})
            if self.instance is None and actions is None:
                raise serializers.ValidationError({'control_actions': '外部 HA 模板必须配置状态检查命令'})
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


class ApplicationDeploymentSerializer(ModelSerializer):
    application_service = serializers.PrimaryKeyRelatedField(
        queryset=ApplicationService.objects.all(), write_only=True, required=False, allow_null=True,
    )
    application_services = serializers.SerializerMethodField()
    application_service_ids = serializers.SerializerMethodField()
    business_system = serializers.SerializerMethodField()
    business_system_name = serializers.SerializerMethodField()
    service_name = serializers.SerializerMethodField()
    topology_type = serializers.SerializerMethodField()
    cluster_type = serializers.SerializerMethodField()
    environment = serializers.SerializerMethodField()
    environment_name = serializers.SerializerMethodField()
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
    ha_role = serializers.SerializerMethodField()

    def get_ha_role(self, obj):
        service = obj.application_services.select_related('cluster_profile').first()
        profile = getattr(service, 'cluster_profile', None) if service else None
        vip = str(getattr(service, 'access_address', '') or '').strip() if service else ''
        if not profile or profile.cluster_type != ClusterProfile.ClusterType.HA or not vip:
            return ApplicationDeployment.HaRole.UNKNOWN
        return (
            ApplicationDeployment.HaRole.PRIMARY
            if str(obj.host.ip or '').strip() == vip
            else ApplicationDeployment.HaRole.STANDBY
        )

    def _first_service(self, obj):
        return obj.application_services.select_related('business_system', 'environment', 'cluster_profile').first()

    def get_application_services(self, obj):
        return [{'id': service.id, 'name': service.name} for service in obj.application_services.all()]

    def get_application_service_ids(self, obj):
        return list(obj.application_services.values_list('id', flat=True))

    def get_business_system(self, obj):
        service = self._first_service(obj)
        return service.business_system_id if service else None

    def get_business_system_name(self, obj):
        service = self._first_service(obj)
        return service.business_system.name if service else None

    def get_service_name(self, obj):
        return self._first_service(obj).name if self._first_service(obj) else None

    def get_topology_type(self, obj):
        service = self._first_service(obj)
        return service.topology_type if service else None

    def get_cluster_type(self, obj):
        service = self._first_service(obj)
        return service.cluster_profile.cluster_type if service and service.cluster_profile else None

    def get_environment(self, obj):
        service = self._first_service(obj)
        return service.environment_id if service else None

    def get_environment_name(self, obj):
        service = self._first_service(obj)
        return service.environment.name if service and service.environment else None

    class Meta:
        model = ApplicationDeployment
        fields = '__all__'

    def _host_bound_business_ids(self, host, *, exclude_deployment=None):
        if host is None:
            return set()
        queryset = ApplicationServiceDeployment.objects.filter(
            deployment__host=host,
        ).exclude(service__business_system_id__isnull=True)
        if exclude_deployment is not None:
            queryset = queryset.exclude(deployment=exclude_deployment)
        return set(queryset.values_list('service__business_system_id', flat=True))

    def validate(self, attrs):
        # 版本与模板只存在于逻辑服务上，实例必须有归属，否则无法得到运行配置。
        service = attrs.get('application_service')
        if self.instance is None and not service:
            raise serializers.ValidationError({'application_service': '请选择部署实例所属的逻辑服务'})
        if service:
            host = attrs.get('host', getattr(self.instance, 'host', None))
            if host is not None:
                if host.environment_id is None:
                    raise serializers.ValidationError({'host': '主机尚未配置所属环境，不能添加部署实例'})
                if service.environment_id != host.environment_id:
                    raise serializers.ValidationError({'host': '主机所属环境必须与逻辑服务环境一致'})
                host_business_ids = self._host_bound_business_ids(host, exclude_deployment=self.instance)
                service_business_id = getattr(service, 'business_system_id', None)
                if host_business_ids and service_business_id not in host_business_ids:
                    raise serializers.ValidationError({'host': '同一主机只能归属一个业务，当前主机已绑定其他业务的部署实例'})
        return attrs

    def create(self, validated_data):
        service = validated_data.pop('application_service', None)
        instance = super().create(validated_data)
        if service:
            ApplicationServiceDeployment.objects.create(service=service, deployment=instance)
        return instance

    def update(self, instance, validated_data):
        service = validated_data.pop('application_service', None)
        instance = super().update(instance, validated_data)
        if service:
            ApplicationServiceDeployment.objects.get_or_create(service=service, deployment=instance)
        return instance


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
    agent_credentials = serializers.SerializerMethodField()
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
    environment_name = serializers.CharField(source='environment.name', read_only=True)

    class Meta:
        model = Host
        fields = '__all__'

    def _host_bound_business_ids(self, host):
        if host is None:
            return set()
        queryset = ApplicationServiceDeployment.objects.filter(
            deployment__host=host,
        ).exclude(service__business_system_id__isnull=True)
        return set(queryset.values_list('service__business_system_id', flat=True))

    def validate(self, attrs):
        instance = getattr(self, 'instance', None)
        target_environment = attrs.get('environment', getattr(instance, 'environment', None))

        if instance is not None:
            host_business_ids = self._host_bound_business_ids(instance)
            if len(host_business_ids) > 1:
                raise serializers.ValidationError({'environment': '当前主机关联了多个业务的历史部署，请先清理后再修改'})

            if host_business_ids:
                if target_environment is None:
                    raise serializers.ValidationError({'environment': '主机已绑定业务部署实例，不能清空所属环境'})
                target_business_id = getattr(target_environment, 'business_system_id', None)
                if target_business_id not in host_business_ids:
                    raise serializers.ValidationError({'environment': '同一主机只能归属一个业务，所属环境与已绑定业务不一致'})

        return attrs

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

    def get_agent_credentials(self, obj):
        """Expose SSH credential choices without returning any secret material."""
        relations = getattr(obj, 'host_credentials', None)
        if relations is None:
            relations = obj.hostcredential_set.select_related('credential').all()
        return [
            {
                'id': relation.credential_id,
                'name': relation.credential.name or f'凭证-{relation.credential_id}',
                'username': relation.credential.username,
                'port': relation.credential.port,
                'auth_type': relation.credential.auth_type,
                'is_default': relation.is_default,
            }
            for relation in relations
            if relation.credential_id and relation.credential
        ]

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
    environment_name = serializers.CharField(source='environment.name', read_only=True)
    agent_credentials = serializers.SerializerMethodField()
    system = serializers.SerializerMethodField()
    hardware = serializers.SerializerMethodField()
    os_type = serializers.SerializerMethodField()
    os_version = serializers.SerializerMethodField()
    kernel_version = serializers.SerializerMethodField()

    class Meta:
        model = Host
        fields = [
            'id',
            'instance_name',
            'agent_id',
            'ip',
            'environment',
            'environment_name',
            'remark',
            'webssh_default_username',
            'webssh_login_users',
            'agent_online',
            'collect_status',
            'agent_credentials',
            'system',
            'hardware',
            'os_type',
            'os_version',
            'kernel_version',
        ]

    def get_agent_credentials(self, obj):
        relations = getattr(obj, 'host_credentials', [])
        return [
            {
                'id': relation.credential_id,
                'name': relation.credential.name or f'凭证-{relation.credential_id}',
                'username': relation.credential.username,
                'port': relation.credential.port,
                'auth_type': relation.credential.auth_type,
                'is_default': relation.is_default,
            }
            for relation in relations
            if relation.credential_id and relation.credential
        ]

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
