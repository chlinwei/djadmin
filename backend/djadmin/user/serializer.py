from rest_framework.serializers import ModelSerializer
from .models import SysUser
class SysUserSerializer(ModelSerializer):
    class Meta:
        model = SysUser
        fields = ["id","username","avatar","email","phonenumber","login_date","status","create_time","update_time","remark"]
        read_only_fields = ['username']