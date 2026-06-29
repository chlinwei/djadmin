from rest_framework import serializers
from sys_config.models import SysConfig


class SysConfigSerializer(serializers.ModelSerializer):
    class Meta:
        model = SysConfig
        fields = ['id', 'key', 'value', 'value_type', 'name', 'description', 'is_readonly', 'create_time', 'update_time']
        read_only_fields = ['id', 'key', 'value_type', 'name', 'is_readonly', 'create_time', 'update_time']
