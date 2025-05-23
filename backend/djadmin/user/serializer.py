from rest_framework.serializers import ModelSerializer
from  rest_framework import serializers
from .models import SysUser
from role.serializer import SysRoleSerializer
from role.models import SysRole
class SysUserSerializer(ModelSerializer):
    class Meta:
        model = SysUser
        fields = ["id","username","avatar","email","phonenumber","login_date","status","create_time","update_time","remark"]
        read_only_fields = ['username']



    

class SysUserRoleSerializer(ModelSerializer):
    roles = serializers.SerializerMethodField()
    def get_roles(self, obj):
        roles = SysRole.objects.filter(sysuserrole__user=obj)
        return SysRoleSerializer(roles, many=True).data
    class Meta:
        model = SysUser
        fields = ["id","username","avatar","email","phonenumber","login_date","status","create_time","update_time","roles","remark"]