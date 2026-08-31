from rest_framework import serializers
from .models import SysRole
from datetime import datetime
class SysRoleSerializer(serializers.ModelSerializer):
    class Meta:
        model = SysRole
        fields = '__all__'
    # 创建角色
    def create(self, validated_data):
        validated_data['create_time'] = datetime.now().date()
        role = SysRole.objects.create(**validated_data)
        return role
