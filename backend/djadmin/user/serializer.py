from rest_framework.serializers import ModelSerializer
from  rest_framework import serializers
from .models import SysUser
from role.serializer import SysRoleSerializer
from role.models import SysRole
from datetime import datetime
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


class StatusSerializer(serializers.Serializer):
    STATUS_CHOICES = (
        (0, '未处理'),
        (1, '已处理')
    )
    status = serializers.ChoiceField(
        choices=STATUS_CHOICES,
        required=True  # 强制要求该字段必须提交
    )
    user_id = serializers.IntegerField(required=True)
    def validate_user_id(self, value):
        if value <= 0:
            raise serializers.ValidationError("用户id必须是正数")
        return value
    


# 用户修改和添加
class UserDetailCreateSerializer(ModelSerializer):
    class Meta:
        model = SysUser
        fields = ["id","username","email","phonenumber","status","remark"]
    # 创建用户
    def create(self, validated_data):
        validated_data["password"] = "123456"
        validated_data["create_time"] = datetime.now().date()
        user = SysUser.objects.create(**validated_data)
        return user
