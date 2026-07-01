from rest_framework import serializers
from .models import SysMenu,SysRoleMenu
from rest_framework.renderers import JSONRenderer
from datetime import datetime

# 菜单树最大层级 — 从 sys_config 动态读取，首次调用自动写入默认值
def _get_max_menu_tree_depth():
    from sys_config.models import SysConfig
    config, _ = SysConfig.objects.get_or_create(
        key='sys.menu.max_tree_depth',
        defaults={
            'value': '5',
            'default_value': '5',
            'value_type': 'int',
            'name': '菜单最大层级',
            'description': '系统菜单树形结构的最大嵌套层数，修改后重启生效',
            'is_readonly': False,
        },
    )
    try:
        return max(1, int(str(config.value).strip()))
    except (ValueError, TypeError):
        return 5


def _get_menu_depth(menu_id):
    """从指定菜单向上追溯，返回其所在层级（根节点 parent_id=0 为第1层）。"""
    depth = 1
    current_id = menu_id
    visited = set()
    while current_id not in (0, None):
        if current_id in visited:
            break  # 防止循环引用
        visited.add(current_id)
        row = SysMenu.objects.filter(id=current_id).values('parent_id').first()
        if not row:
            break
        current_id = row['parent_id']
        depth += 1
    return depth

class SysMenuSerializer(serializers.ModelSerializer):
    children = serializers.SerializerMethodField()
    def get_children(self, obj):
        if hasattr(obj, "children"):
            serializerMenuList: list[SysMenuSerializer2] = list()
            for sysMenu in obj.children:
                serializerMenuList.append(SysMenuSerializer2(sysMenu).data)  # type: ignore[arg-type]
            return serializerMenuList
    class Meta:
        model = SysMenu
        fields = '__all__'


class SysMenuDynamicListSerializer(serializers.BaseSerializer):
    @staticmethod
    def listToTreeList(menuList):
        treeList = []
        for menu in menuList:
            if menu.parent_id == 0:
                treeList.append(SysMenuDynamicListSerializer.findChildren(menu,menuList))
        return treeList
    
    @staticmethod
    def findChildren(node, menuList):
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
            'remark':node.remark,
            'location':node.location,
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

    def validate(self, attrs):
        parent_id = attrs.get('parent_id')
        # 编辑时若未传 parent_id，从实例取
        if parent_id is None and self.instance:
            parent_id = self.instance.parent_id
        if parent_id not in (0, None, ''):
            max_depth = _get_max_menu_tree_depth()
            depth = _get_menu_depth(int(parent_id))
            if depth >= max_depth:
                raise serializers.ValidationError(
                    {"parent_id": [f"菜单层级不能超过 {max_depth} 层"]}
                )
        return attrs

    # 创建
    def create(self, validated_data):
        validated_data["create_time"] = datetime.now().date()
        menu = SysMenu.objects.create(**validated_data)
        return menu





class SysRoleMenuSerializer(serializers.ModelSerializer):
    class Meta:
        model = SysRoleMenu
        fields = '__all__'