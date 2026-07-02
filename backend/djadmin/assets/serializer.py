from rest_framework.serializers import ModelSerializer
from datetime import datetime
from .models import *
from rest_framework import serializers



# host user
class CredentialSerializer(ModelSerializer):
    class Meta:
        model = Credential
        fields = '__all__'
    
    # 创建
    def create(self, validated_data):
        validated_data["create_time"] = datetime.now().date()
        data = Credential.objects.create(**validated_data)
        return data
    def validate_private_key(self, value):
        if value and len(value) > 8192:
            raise serializers.ValidationError("私钥长度超过限制")
        return value
# host type
class ApplicationSerializer(ModelSerializer):
    class Meta:
        model = Application
        fields = '__all__'
    
    # 创建
    def create(self, validated_data):
        validated_data["create_time"] = datetime.now().date()
        data = Application.objects.create(**validated_data)
        return data


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
        validated_data["create_time"] = datetime.now().date()
        data = HostGroup.objects.create(**validated_data)
        return data

    def update(self, instance, validated_data):
        parent_id = validated_data.pop("parent_id", instance.parent_id)
        parent_id = self._validate_parent_depth(parent_id)
        instance.parent_id = parent_id
        instance.name = validated_data.get("name", instance.name)
        instance.remark = validated_data.get("remark", instance.remark)
        instance.update_time = datetime.now().date()
        instance.save()
        return instance

# host 
class HostSerializer(ModelSerializer):
    group_id = serializers.IntegerField(required=False, allow_null=True, write_only=True)
    credential_id = serializers.IntegerField(required=False, allow_null=True, write_only=True)
    group_name = serializers.SerializerMethodField()
    credential = serializers.SerializerMethodField()
    system = serializers.SerializerMethodField()
    hardware = serializers.SerializerMethodField()
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

    class Meta:
        model = Host
        fields = '__all__'

    def _get_default_host_credential(self, obj):
        cached_credentials = getattr(obj, 'hostcredential_set', None)
        if cached_credentials is not None:
            for relation in cached_credentials.all():
                if relation.is_default and relation.credential:
                    return relation
        return HostCredential.objects.filter(host=obj, is_default=True).select_related('credential').first()

    def get_group_name(self, obj):
        return obj.group.name if obj.group else ''

    def get_credential(self, obj):
        relation = self._get_default_host_credential(obj)
        if not relation or not relation.credential:
            return None
        credential = relation.credential
        return {
            'id': credential.id,
            'name': credential.name,
            'username': credential.username,
            'port': credential.port,
            'auth_type': credential.auth_type,
        }

    def get_system(self, obj):
        system = getattr(obj, 'system', None)
        if not system:
            return None
        return {
            'os_type': system.os_type,
            'os_version': system.os_version,
            'kernel_version': system.kernel_version,
            'hostname': system.hostname,
            'agent_version': system.agent_version,
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
        candidates = []
        system = getattr(obj, 'system', None)
        hardware = getattr(obj, 'hardware', None)

        if system and system.collected_at:
            candidates.append(system.collected_at)
        if hardware and hardware.collected_at:
            candidates.append(hardware.collected_at)

        # fallback for historical rows before collected_at field exists
        if not candidates:
            if system and system.update_time:
                candidates.append(system.update_time)
            if hardware and hardware.update_time:
                candidates.append(hardware.update_time)

        if not candidates:
            return None

        return max(candidates)
    
    # 创建
    def create(self, validated_data):
        group_id = validated_data.pop('group_id', None)
        credential_id = validated_data.pop('credential_id', None)
        if group_id in (0, '0', '', None):
            group_id = None
        validated_data['group_id'] = group_id
        validated_data["create_time"] = datetime.now().date()
        data = Host.objects.create(**validated_data)
        self._sync_default_credential(data, credential_id)
        return data

    def _sync_default_credential(self, host, credential_id):
        HostCredential.objects.filter(host=host).delete()
        if credential_id in (0, '0', '', None):
            return
        HostCredential.objects.create(host=host, credential_id=credential_id, is_default=True)

    def update(self, instance, validated_data):
        group_id = validated_data.pop('group_id', serializers.empty)
        credential_id = validated_data.pop('credential_id', serializers.empty)

        if group_id is not serializers.empty:
            if group_id in (0, '0', '', None):
                instance.group_id = None
            else:
                instance.group_id = group_id

        if credential_id is not serializers.empty:
            HostCredential.objects.filter(host=instance).delete()
            if credential_id not in (0, '0', '', None):
                HostCredential.objects.create(host=instance, credential_id=credential_id, is_default=True)

        for attr, value in validated_data.items():
            setattr(instance, attr, value)

        instance.update_time = datetime.now().date()
        instance.save()
        return instance


class WebSSHSessionLogSerializer(ModelSerializer):
    host_name = serializers.SerializerMethodField()
    host_ip = serializers.SerializerMethodField()

    class Meta:
        model = WebSSHSessionLog
        fields = [
            'id',
            'session_id',
            'host',
            'host_name',
            'host_ip',
            'user_id',
            'username',
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
        return obj.host.instance_name or obj.host.name

    def get_host_ip(self, obj):
        return obj.host.ip


