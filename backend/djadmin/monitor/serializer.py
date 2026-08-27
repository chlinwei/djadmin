from rest_framework import serializers
from rest_framework.serializers import ModelSerializer

import hashlib
import re

from assets.credential_crypto import encrypt_secret

from .models import (
    AlertHistory,
    AlertMedia,
    AlertRoute,
    LogCollectionTarget,
    LogProcessingRule,
    MonitorTarget,
    MonitorTargetInstallHistory,
    OpenSearchCluster,
    SoftwarePackage,
)


class LogProcessingRuleSerializer(ModelSerializer):
    referenced_log_count = serializers.IntegerField(source='log_definitions.count', read_only=True)

    class Meta:
        model = LogProcessingRule
        fields = '__all__'

    def validate_name(self, value):
        name = str(value or '').strip()
        if not re.fullmatch(r'[a-z0-9][a-z0-9._-]*', name):
            raise serializers.ValidationError('仅支持小写字母、数字、点、下划线和连字符')
        if self.instance is not None and name != self.instance.name:
            raise serializers.ValidationError('规则名称发布后不可修改')
        return name

    def validate_pipeline_body(self, value):
        if not isinstance(value, dict):
            raise serializers.ValidationError('Pipeline 必须是 JSON 对象')
        if not isinstance(value.get('processors'), list):
            raise serializers.ValidationError('Pipeline processors 必须是数组')
        return value

    def validate(self, attrs):
        multiline_enabled = attrs.get(
            'multiline_enabled', getattr(self.instance, 'multiline_enabled', False),
        )
        start_pattern = str(attrs.get('start_pattern', getattr(self.instance, 'start_pattern', '')) or '').strip()
        continuation_pattern = str(
            attrs.get('continuation_pattern', getattr(self.instance, 'continuation_pattern', '')) or ''
        ).strip()
        flush_timeout = attrs.get('flush_timeout', getattr(self.instance, 'flush_timeout', 1000))
        if multiline_enabled and (not start_pattern or not continuation_pattern):
            raise serializers.ValidationError({'multiline': '启用多行合并时必须填写首行和续行正则'})
        if any('\n' in value or '\r' in value for value in (start_pattern, continuation_pattern)):
            raise serializers.ValidationError({'multiline': '多行正则不能包含换行符'})
        if not 100 <= flush_timeout <= 60000:
            raise serializers.ValidationError({'flush_timeout': '合并超时必须在 100 到 60000 毫秒之间'})
        attrs['start_pattern'] = start_pattern if multiline_enabled else ''
        attrs['continuation_pattern'] = continuation_pattern if multiline_enabled else ''
        return attrs


class MonitorTargetSerializer(ModelSerializer):
    target_type = serializers.SerializerMethodField()
    host_name = serializers.SerializerMethodField()
    host_ip = serializers.SerializerMethodField()
    host_agent_online = serializers.SerializerMethodField()

    class Meta:
        model = MonitorTarget
        fields = '__all__'

    def get_target_type(self, obj):
        return SoftwarePackage.PackageType.EXPORTER

    def get_host_name(self, obj):
        host = getattr(obj, 'host', None)
        if host is None:
            return ''
        return str(getattr(host, 'instance_name', '') or '')

    def get_host_ip(self, obj):
        host = getattr(obj, 'host', None)
        if host is None:
            return ''
        return str(getattr(host, 'ip', '') or '')

    def get_host_agent_online(self, obj):
        host = getattr(obj, 'host', None)
        if host is None:
            return False
        return bool(getattr(host, 'agent_online', False))

    def validate_scrape_port(self, value):
        port = int(value)
        if port < 1 or port > 65535:
            raise serializers.ValidationError('scrape_port must be between 1 and 65535')
        return port


class SoftwarePackageSerializer(ModelSerializer):
    download_url = serializers.SerializerMethodField()
    file_name = serializers.SerializerMethodField()
    synced = serializers.SerializerMethodField()
    install_playbook_template_name = serializers.SerializerMethodField()
    uninstall_playbook_template_name = serializers.SerializerMethodField()

    class Meta:
        model = SoftwarePackage
        fields = '__all__'
        validators = []
        # sha256/size 由上传文件自动计算，不接受客户端传入
        # install/uninstall_playbook_template 自 2026-07 起不再由前端直接选择已存在模板 id 写入，
        # 改为通过 install_playbook_content/uninstall_playbook_content（非模型字段，见 to_representation/
        # _sync_playbook_template）行内提交内容，由后端自动创建/更新/解绑底层 PlaybookTemplate，
        # 因此这两个字段这里设为只读，仅用于展示当前绑定的模板 id。
        read_only_fields = (
            'sha256', 'size_bytes', 'create_time', 'update_time',
            'install_playbook_template', 'uninstall_playbook_template',
        )
        # 模型层 file 允许为空（blank=True）：支持先创建“未同步”占位记录（与 ensure_defaults
        # 预置行同一语义），后续经行内上传补全文件，因此这里不再强制 required=True。
        # service_run_as_user 模型层设了 default='dj-agent'，DRF 默认会因此把它推断为非必填
        # （ModelSerializer 对有 default 的字段自动 required=False），这里显式覆盖为必填，
        # 确保前端/接口层都强制要求填写运行用户。
        extra_kwargs = {
            'service_run_as_user': {'required': True, 'allow_blank': False},
        }

    def get_download_url(self, obj):
        file_field = getattr(obj, 'file', None)
        if not file_field:
            return ''
        try:
            return file_field.url
        except ValueError:
            return ''

    def get_file_name(self, obj):
        file_field = getattr(obj, 'file', None)
        if not file_field or not file_field.name:
            return ''
        return file_field.name.rsplit('/', 1)[-1]

    def get_synced(self, obj):
        return bool(getattr(obj, 'file', None) and obj.file.name and obj.sha256)

    def get_install_playbook_template_name(self, obj):
        template = getattr(obj, 'install_playbook_template', None)
        return str(getattr(template, 'name', '') or '') if template is not None else ''

    def get_uninstall_playbook_template_name(self, obj):
        template = getattr(obj, 'uninstall_playbook_template', None)
        return str(getattr(template, 'name', '') or '') if template is not None else ''

    def validate_default_port(self, value):
        port = int(value)
        if port < 1 or port > 65535:
            raise serializers.ValidationError('default_port must be between 1 and 65535')
        return port

    def validate(self, attrs):
        package_type = attrs.get('package_type', getattr(self.instance, 'package_type', 'exporter'))
        name = str(attrs.get('name', getattr(self.instance, 'name', '')) or '').strip().lower()
        if package_type == SoftwarePackage.PackageType.FLUENT_BIT and name != 'fluent-bit':
            raise serializers.ValidationError({'name': 'Fluent Bit 类型的软件包名称必须为 fluent-bit'})
        package_format = attrs.get('package_format', getattr(self.instance, 'package_format', 'tar.gz'))
        platform_family = attrs.get('platform_family', getattr(self.instance, 'platform_family', 'any'))
        platform_major = str(
            attrs.get('platform_major', getattr(self.instance, 'platform_major', '')) or ''
        ).strip()
        if package_format == SoftwarePackage.PackageFormat.TAR_GZ:
            if platform_family != SoftwarePackage.PlatformFamily.ANY or platform_major:
                raise serializers.ValidationError({
                    'platform_family': '通用 tar.gz 必须选择“通用 Linux”且主版本留空',
                })
        elif package_format == SoftwarePackage.PackageFormat.RPM:
            if platform_family != SoftwarePackage.PlatformFamily.RHEL or not platform_major:
                raise serializers.ValidationError({
                    'platform_family': 'RPM 必须选择 RHEL 平台族并填写主版本（7/8/9）',
                })
        elif package_format == SoftwarePackage.PackageFormat.DEB:
            if platform_family not in {
                SoftwarePackage.PlatformFamily.UBUNTU,
                SoftwarePackage.PlatformFamily.DEBIAN,
            } or not platform_major:
                raise serializers.ValidationError({
                    'platform_family': 'DEB 必须选择 Ubuntu/Debian 并填写主版本',
                })
        attrs['platform_major'] = platform_major
        unique_values = {
            field: attrs.get(field, getattr(self.instance, field, None))
            for field in ('name', 'version', 'os', 'arch')
        }
        duplicate_query = SoftwarePackage.objects.filter(
            **unique_values,
            package_type=package_type,
            platform_family=platform_family,
            platform_major=platform_major,
        )
        if self.instance is not None:
            duplicate_query = duplicate_query.exclude(pk=self.instance.pk)
        if duplicate_query.exists():
            raise serializers.ValidationError('相同版本、系统、架构和平台的安装包已存在')
        return attrs

    @staticmethod
    def _get_template_content(template):
        return str(getattr(template, 'content', '') or '') if template is not None else ''

    def to_representation(self, instance):
        # install/uninstall_playbook_content 不是模型字段（内容实际存放在关联的 PlaybookTemplate.content
        # 上），这里手动补充到输出里，供前端软件包编辑弹窗内联展示/编辑。
        data = super().to_representation(instance)
        data['install_playbook_content'] = self._get_template_content(instance.install_playbook_template)
        data['uninstall_playbook_content'] = self._get_template_content(instance.uninstall_playbook_template)
        return data

    def _sync_playbook_template(self, instance, role, raw_content):
        """把行内编辑提交的 Playbook 内容同步为对应角色（install/uninstall）的底层 PlaybookTemplate 记录。
        raw_content 为 None 表示本次请求未提交该字段（如仅编辑其他元数据的 partial_update），保持原样不变；
        空字符串表示用户主动清空，此时解绑并删除自动管理的旧模板。模板 category 固定为 software_package，
        名称按 `<name>-<id>-<role>` 拼接以保证唯一（同名软件包可能存在多个 os/arch 行），用户无需感知。
        """
        if raw_content is None:
            return
        fk_field = f'{role}_playbook_template'
        content = str(raw_content or '').strip()
        current_template = getattr(instance, fk_field)

        if content == '':
            if current_template is not None:
                setattr(instance, fk_field, None)
                instance.save(update_fields=[fk_field, 'update_time'])
                current_template.delete()
            return

        if current_template is not None:
            current_template.content = content
            current_template.save(update_fields=['content', 'update_time'])
            return

        from automation.models import PlaybookTemplate, TemplateCategory
        template = PlaybookTemplate.objects.create(
            name=f'{instance.name}-{instance.id}-{role}',  # type: ignore[attr-defined]
            content=content,
            category=TemplateCategory.SOFTWARE_PACKAGE,
        )
        setattr(instance, fk_field, template)
        instance.save(update_fields=[fk_field, 'update_time'])

    def _fill_file_meta(self, validated_data):
        # 上传/替换文件时，流式计算 sha256 并记录字节数，供 agent 下载后完整性校验
        upload_file = validated_data.get('file')
        if upload_file is None:
            return
        hasher = hashlib.sha256()
        for chunk in upload_file.chunks():
            hasher.update(chunk)
        upload_file.seek(0)
        validated_data['sha256'] = hasher.hexdigest()
        validated_data['size_bytes'] = int(getattr(upload_file, 'size', 0) or 0)

    def create(self, validated_data):
        self._fill_file_meta(validated_data)
        instance = super().create(validated_data)
        # self.initial_data 在 create/update 调用时（is_valid() 已通过）必为请求携带的 dict/QueryDict，
        # DRF 类型存根把 initial_data 声明为可能是 `empty` 导致 Pylance 误报，属已知误报。
        self._sync_playbook_template(instance, 'install', self.initial_data.get('install_playbook_content'))  # type: ignore[union-attr]
        self._sync_playbook_template(instance, 'uninstall', self.initial_data.get('uninstall_playbook_content'))  # type: ignore[union-attr]
        instance.refresh_from_db()
        return instance

    def update(self, instance, validated_data):
        self._fill_file_meta(validated_data)
        instance = super().update(instance, validated_data)
        self._sync_playbook_template(instance, 'install', self.initial_data.get('install_playbook_content'))  # type: ignore[union-attr]
        self._sync_playbook_template(instance, 'uninstall', self.initial_data.get('uninstall_playbook_content'))  # type: ignore[union-attr]
        instance.refresh_from_db()
        return instance


class MonitorTargetInstallHistorySerializer(ModelSerializer):
    host_name = serializers.SerializerMethodField()
    host_ip = serializers.SerializerMethodField()
    target_exporter_type = serializers.SerializerMethodField()
    managed_target_id = serializers.SerializerMethodField()
    target_type = serializers.SerializerMethodField()

    class Meta:
        model = MonitorTargetInstallHistory
        fields = '__all__'

    def get_host_name(self, obj):
        host = getattr(obj, 'host', None)
        if host is not None:
            return str(getattr(host, 'instance_name', '') or '')
        return str(getattr(obj, 'host_name_snapshot', '') or '')

    def get_host_ip(self, obj):
        host = getattr(obj, 'host', None)
        if host is not None:
            return str(getattr(host, 'ip', '') or '')
        return str(getattr(obj, 'host_ip_snapshot', '') or '')

    def get_target_exporter_type(self, obj):
        target = getattr(obj, 'target', None)
        if target is not None:
            return str(getattr(target, 'exporter_type', '') or '')
        return str(getattr(obj, 'exporter_type_snapshot', '') or '')

    def get_managed_target_id(self, obj):
        return getattr(obj, 'target_id', None) or getattr(obj, 'log_collection_target_id', None)

    def get_target_type(self, obj):
        return 'fluent_bit' if getattr(obj, 'log_collection_target_id', None) else 'exporter'


class AlertHistorySerializer(ModelSerializer):
    notification_count = serializers.IntegerField(read_only=True, default=0)
    notification_delivery_count = serializers.IntegerField(read_only=True, default=0)
    notification_status = serializers.SerializerMethodField()
    rule_group = serializers.SerializerMethodField()
    rule_details = serializers.SerializerMethodField()

    @staticmethod
    def get_notification_status(obj):
        notification_count = int(getattr(obj, 'notification_count', 0) or 0)
        if notification_count == 0:
            return 'none'
        failed_count = int(getattr(obj, 'notification_failed_count', 0) or 0)
        active_count = int(getattr(obj, 'notification_active_count', 0) or 0)
        delivery_count = int(getattr(obj, 'notification_delivery_count', 0) or 0)
        if failed_count > 0 or (active_count == 0 and delivery_count == 0):
            return 'failed'
        if active_count > 0:
            return 'in_progress'
        return 'success'

    def get_rule_group(self, obj):
        return str(getattr(obj, 'rule_group', '') or '').strip()

    def get_rule_details(self, obj):
        stored_rule_snapshot = getattr(obj, 'rule_snapshot', None)
        if isinstance(stored_rule_snapshot, dict) and stored_rule_snapshot:
            return stored_rule_snapshot
        return {}

    class Meta:
        model = AlertHistory
        fields = '__all__'


class AlertMediaSerializer(ModelSerializer):
    class Meta:
        model = AlertMedia
        fields = [
            'id', 'name', 'media_type', 'config', 'enabled',
            'create_time', 'update_time', 'remark',
        ]
        read_only_fields = ['id', 'create_time', 'update_time']

    def validate(self, attrs):
        media_type = attrs.get('media_type', getattr(self.instance, 'media_type', None))
        if media_type != AlertMedia.MediaType.EMAIL:
            raise serializers.ValidationError({'media_type': '当前仅支持 Email 媒介'})

        config = dict(attrs.get('config') or getattr(self.instance, 'config', {}) or {})
        if config.get('provider') == 'gmail':
            config['smtpServer'] = 'smtp.gmail.com'
            config['smtpPort'] = 587
            config['authType'] = 'password'
        provider = str(config.get('provider') or '').strip().lower()
        default_auth_type = 'none' if provider == 'custom' else 'password'
        auth_type = str(config.get('authType') or default_auth_type).strip().lower()
        if auth_type not in {'none', 'password'}:
            raise serializers.ValidationError({'config': {'authType': '认证方式必须是 none 或 password'}})
        config['authType'] = auth_type
        if auth_type == 'none':
            config.pop('password', None)
        is_custom_smtp = config.get('provider') == 'custom'
        required_fields = {
            'smtpServer': 'SMTP服务器不能为空',
            'smtpPort': 'SMTP服务器端口不能为空',
            'email': '电子邮件不能为空',
        }
        if auth_type == 'password':
            if is_custom_smtp:
                required_fields['username'] = '用户名不能为空'
            required_fields['password'] = '密码不能为空'
        errors = {key: message for key, message in required_fields.items() if not config.get(key)}
        if errors:
            raise serializers.ValidationError({'config': errors})
        if auth_type == 'none' or not is_custom_smtp:
            config.pop('username', None)
        try:
            smtp_port = int(config['smtpPort'])
        except (TypeError, ValueError):
            raise serializers.ValidationError({'config': {'smtpPort': 'SMTP服务器端口必须是整数'}})
        if not 1 <= smtp_port <= 65535:
            raise serializers.ValidationError({'config': {'smtpPort': 'SMTP服务器端口必须在 1-65535 之间'}})
        config['smtpPort'] = smtp_port
        attrs['config'] = config
        return attrs

    @staticmethod
    def _encrypted_config(config):
        result = dict(config or {})
        if result.get('password') and result.get('password') != '********':
            result['password'] = encrypt_secret(result['password'])
        return result

    def create(self, validated_data):
        validated_data['config'] = self._encrypted_config(validated_data.get('config'))
        return super().create(validated_data)

    def update(self, instance, validated_data):
        if 'config' in validated_data:
            incoming = dict(validated_data['config'] or {})
            if incoming.get('password') == '********':
                current = instance.config if isinstance(instance.config, dict) else {}
                incoming['password'] = current.get('password', '')
            validated_data['config'] = self._encrypted_config(incoming)
        return super().update(instance, validated_data)

    def to_representation(self, instance):
        data = super().to_representation(instance)
        config = dict(data.get('config') or {})
        if config.get('password'):
            config['password'] = '********'
        data['config'] = config
        return data

    @staticmethod
    def _encrypted_config(config):
        result = dict(config or {})
        if result.get('password') and result.get('password') != '********':
            result['password'] = encrypt_secret(result['password'])
        return result

    def create(self, validated_data):
        validated_data['config'] = self._encrypted_config(validated_data.get('config'))
        return super().create(validated_data)

    def update(self, instance, validated_data):
        if 'config' in validated_data:
            incoming = dict(validated_data['config'] or {})
            if incoming.get('password') == '********':
                current = instance.config if isinstance(instance.config, dict) else {}
                incoming['password'] = current.get('password', '')
            validated_data['config'] = self._encrypted_config(incoming)
        return super().update(instance, validated_data)

    def to_representation(self, instance):
        data = super().to_representation(instance)
        config = dict(data.get('config') or {})
        if config.get('password'):
            config['password'] = '********'
        data['config'] = config
        return data


class AlertRouteSerializer(ModelSerializer):
    media = serializers.PrimaryKeyRelatedField(
        queryset=AlertMedia.objects.all(), many=True,
    )

    class Meta:
        model = AlertRoute
        fields = [
            'id', 'name', 'enabled', 'matchers', 'notify_on_firing',
            'notify_on_resolved', 'media', 'remark', 'create_time', 'update_time',
        ]
        read_only_fields = ['id', 'create_time', 'update_time']

    def validate(self, attrs):
        firing = attrs.get('notify_on_firing', getattr(self.instance, 'notify_on_firing', True))
        resolved = attrs.get('notify_on_resolved', getattr(self.instance, 'notify_on_resolved', True))
        if not firing and not resolved:
            raise serializers.ValidationError('至少选择一种通知事件类型')

        matchers = attrs.get('matchers', getattr(self.instance, 'matchers', {}))
        if not isinstance(matchers, dict):
            raise serializers.ValidationError({'matchers': '标签匹配条件必须是对象'})
        normalized_matchers = {}
        for key, value in matchers.items():
            normalized_key = str(key).strip()
            normalized_value = str(value).strip()
            if not normalized_key or not normalized_value:
                raise serializers.ValidationError({'matchers': '标签名和值不能为空'})
            normalized_matchers[normalized_key] = normalized_value
        attrs['matchers'] = normalized_matchers

        media = attrs.get('media')
        if media is not None and not media:
            raise serializers.ValidationError({'media': '至少选择一个通知媒介'})
        return attrs


class UserAlertMediaBindingSerializer(ModelSerializer):
    """用户媒介绑定序列化器：处理用户在某个媒介上的收件人配置。"""
    
    media_name = serializers.CharField(source='media.name', read_only=True)
    media_type = serializers.CharField(source='media.media_type', read_only=True)

    class Meta:
        model = serializers.ModelSerializer
        fields = [
            'id', 'media', 'media_name', 'media_type', 'recipients', 'enabled',
            'create_time', 'update_time', 'remark',
        ]
        read_only_fields = ['id', 'create_time', 'update_time']

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        from .models import UserAlertMediaBinding
        self.Meta.model = UserAlertMediaBinding

    def validate(self, attrs):
        """验证并规范化收件人。"""
        recipients_input = attrs.get('recipients', getattr(self.instance, 'recipients', []) if self.instance else [])
        recipients = self._normalize_recipients(recipients_input)
        if not recipients:
            raise serializers.ValidationError({'recipients': '至少需要指定一个收件人邮箱'})
        attrs['recipients'] = recipients
        return attrs

    @staticmethod
    def _normalize_recipients(recipients_input):
        """将收件人输入规范化为列表；支持字符串（逗号/分号分隔）和列表两种格式。"""
        if isinstance(recipients_input, str):
            recipients_input = recipients_input.replace(';', ',').split(',')
        if not isinstance(recipients_input, list):
            return []
        result = []
        for item in recipients_input:
            email = str(item).strip() if item else ''
            if email and '@' in email:
                if email not in result:
                    result.append(email)
        return result


# 提交该占位符表示不修改已保存的密码，避免编辑时未改动就把密文覆盖成占位符本身。
PASSWORD_MASK = '******'


class OpenSearchClusterSerializer(ModelSerializer):
    password = serializers.CharField(required=False, allow_blank=True, write_only=True)
    password_configured = serializers.SerializerMethodField()

    class Meta:
        model = OpenSearchCluster
        fields = '__all__'
        read_only_fields = ['last_check_time', 'last_check_success', 'last_check_message']

    def get_password_configured(self, obj):
        return bool(obj.password)

    def validate_hosts(self, value):
        hosts = [item.strip() for item in str(value or '').split(',') if item.strip()]
        if not hosts:
            raise serializers.ValidationError('至少配置一个连接地址')
        for host in hosts:
            if not host.startswith(('http://', 'https://')):
                raise serializers.ValidationError(f'地址必须以 http:// 或 https:// 开头: {host}')
        return ','.join(hosts)

    def validate_index_prefix(self, value):
        prefix = str(value or '').strip()
        if not prefix:
            raise serializers.ValidationError('索引前缀不能为空')
        if prefix != prefix.lower() or ' ' in prefix:
            raise serializers.ValidationError('索引前缀必须为小写且不含空格')
        return prefix

    @staticmethod
    def _handle_password(validated_data):
        if 'password' not in validated_data:
            return
        raw = validated_data.get('password')
        if raw == PASSWORD_MASK:
            validated_data.pop('password')
            return
        validated_data['password'] = encrypt_secret(raw)

    def create(self, validated_data):
        self._handle_password(validated_data)
        instance = super().create(validated_data)
        self._sync_default_flag(instance)
        return instance

    def update(self, instance, validated_data):
        self._handle_password(validated_data)
        instance = super().update(instance, validated_data)
        self._sync_default_flag(instance)
        return instance

    @staticmethod
    def _sync_default_flag(instance):
        """默认集群全局唯一，设为默认时清掉其他记录的标记。"""
        if instance.is_default:
            OpenSearchCluster.objects.exclude(pk=instance.pk).filter(is_default=True).update(is_default=False)


class LogCollectionTargetSerializer(ModelSerializer):
    target_type = serializers.SerializerMethodField()
    host_name = serializers.CharField(source='host.instance_name', read_only=True, default='')
    host_ip = serializers.CharField(source='host.ip', read_only=True, default='')
    host_agent_online = serializers.BooleanField(source='host.agent_online', read_only=True, default=False)

    class Meta:
        model = LogCollectionTarget
        fields = '__all__'

    def get_target_type(self, obj):
        return SoftwarePackage.PackageType.FLUENT_BIT
