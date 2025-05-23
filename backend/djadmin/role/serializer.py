from rest_framework import serializers
from .models import SysRole

class SysRoleSerializer(serializers.ModelSerializer):
    class Meta:
        model = SysRole
        fields = '__all__'


# class SysUserRoleSerializer(serializers.ModelSerializer):
#     class Meta:
#         model = SysUserRole
#         fields = '__amll__'
