from rest_framework import serializers
from .models import SysMenu,SysRoleMenu
from rest_framework.renderers import JSONRenderer
from datetime import datetime
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


class SysMenuDynamicListSerializer(serializers.BaseSerializer):
    def listToTreeList(menuList):
        treeList = []
        for menu in menuList:
            if menu.parent_id == 0:
                treeList.append(SysMenuDynamicListSerializer.findChildren(menu,menuList))
        return treeList
    
    def findChildren(node,menuList):
        dict_menu = {
            'id': node.id,
            'name': node.name,
            'icon': node.icon,
            'parent_id': node.parent_id,
            'order_num': node. order_num,
            'path': node.path,
            'component': node.component,
            'menu_type': node.menu_type,
            'perms': node.perms,
            'create_time':node.create_time,
            'update_time':node.update_time,
            'remark':node.remark
        }
        for menu in menuList:
            if menu.parent_id == dict_menu['id']:
                if 'children' not in dict_menu:
                    dict_menu['children'] = []
                dict_menu['children'].append(SysMenuDynamicListSerializer.findChildren(menu,menuList))
        return dict_menu
    

    def to_representation(self, instance):
        tmpList = SysMenuDynamicListSerializer.listToTreeList(instance)
        return tmpList
    


class SysMenuSerializer2(serializers.ModelSerializer):
    class Meta:
        model = SysMenu
        fields = '__all__'
    
    # 创建
    def create(self, validated_data):
        print("create")
        print(validated_data)
        validated_data["create_time"] = datetime.now().date()
        menu = SysMenu.objects.create(**validated_data)
        return menu





class SysRoleMenuSerializer(serializers.ModelSerializer):
    class Meta:
        model = SysRoleMenu
        fields = '__all__'