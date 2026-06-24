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
    MAX_TREE_DEPTH = 5
    parent_id = serializers.IntegerField(required=False, allow_null=True)
    parent_name = serializers.SerializerMethodField()
    host_count = serializers.SerializerMethodField()

    class Meta:
        model = HostGroup
        fields = '__all__'

    def get_parent_name(self, obj):
        if obj.parent:
            return obj.parent.name
        return ''

    def get_host_count(self, obj):
        return obj.host_set.count()

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
            raise serializers.ValidationError({"parent_id": "上级分组不存在"})
        if self._get_group_depth(parent) >= self.MAX_TREE_DEPTH:
            raise serializers.ValidationError({"parent_id": f"分组层级不能超过{self.MAX_TREE_DEPTH}层"})
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
            'architecture': hardware.architecture,
        }

    def get_disks(self, obj):
        return [
            {
                'device': disk.device,
                'mount_point': disk.mount_point,
                'size_gb': disk.size_gb,
                'used_gb': disk.used_gb,
                'filesystem': disk.filesystem,
            }
            for disk in obj.disks.all()
        ]
    
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


