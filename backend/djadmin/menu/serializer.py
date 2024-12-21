from rest_framework import serializers
from .models import SysMenu,SysRoleMenu
class SysMenuSerializer(serializers.ModelSerializer):
    children = serializers.SerializerMethodField()
    def get_children(self, obj):
        if hasattr(obj, "children"):
            serializerMenuList: list[SysMenuSerializer2] = list()
            for sysMenu in obj.children:
                serializerMenuList.append(SysMenuSerializer2(sysMenu).data)
            return serializerMenuList
    class Meta:
        model = SysMenu
        fields = '__all__'


class SysMenuSerializer2(serializers.ModelSerializer):
    class Meta:
        model = SysMenu
        fields = '__all__'



class SysRoleMenuSerializer(serializers.ModelSerializer):
    class Meta:
        model = SysRoleMenu
        fields = '__all__'