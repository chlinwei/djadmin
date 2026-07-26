from rest_framework import serializers
from sys_config.models import SysConfig


class SysConfigSerializer(serializers.ModelSerializer):
    class Meta:
        model = SysConfig
        fields = ['id', 'key', 'value', 'default_value', 'value_type', 'name', 'description', 'is_readonly', 'create_time', 'update_time']
        read_only_fields = ['id', 'key', 'value_type', 'name', 'is_readonly', 'create_time', 'update_time']

    def to_representation(self, instance):
        # secret 类型的 value 字段落库的是哈希，任何对外响应都必须走 get_typed_value()
        # 走掩码占位符，禁止把哈希原文暴露给前端（哈希本身也是不该外泄的敏感信息）。
        data = super().to_representation(instance)
        if instance.value_type == 'secret':
            data['value'] = instance.get_typed_value()
        return data
