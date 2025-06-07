from .models import SysMenu,SysRoleMenu
from .models import SysMenu
from .models import SysRoleMenu
from .serializer import SysMenuDynamicListSerializer,SysMenuSerializer2
from djadmin.utils import Response_200
from rest_framework.mixins import CreateModelMixin,UpdateModelMixin,RetrieveModelMixin
from rest_framework.viewsets import GenericViewSet
from rest_framework.decorators import action
from menu.permisssion import CustomMenuPermission

# 自带新增,更新
class MenuManage(GenericViewSet,CreateModelMixin,UpdateModelMixin,RetrieveModelMixin):
    permission_classes = [CustomMenuPermission]
    action_perms_map = {
        # 查看
        'getMenuTree': 'system:roles:list',
        'retrieve': 'system:roles:list',
        'getMenuListByRoleId':'system:roles:list',
        # 修改
        'partial_update': 'system:menus:update',
        'perform_update': 'system:menus:update',
        # 新增
        'create': 'system:menus:create',   
        # 删除
        'deleteMenuById':'system:menus:delete', 
        
    }
    queryset = SysMenu.objects.all()
    serializer_class = SysMenuSerializer2
    lookup_field = 'id'
    @action(detail=False,methods=['get'])
    # 获取权限树
    def getMenuTree(self,request,url_path="getMenuTree"):
        menu_list = SysMenu.objects.order_by("order_num")
        menu_list_data = SysMenuDynamicListSerializer(menu_list)
        return Response_200(menu_list_data.data)
    # 获取一个角色所包含的权限列表
    @action(detail=False,methods=['get'],url_path="getMenuListByRoleId")
    def getMenuListByRoleId(self,request):
        role_id = request.query_params.get("role_id")
        print(role_id)
        menuList = SysRoleMenu.objects.filter(role_id=role_id).values("menu_id")
        menuIdList = [m['menu_id'] for m in menuList]
        return Response_200(menuIdList)
    
    @action(detail=False,methods=['post'],url_path="grantMenu")
    def grantMenu(self,request):
        role_id = request.data.get("role_id")
        menuIdList = request.data.get('menuIds')
        SysRoleMenu.objects.filter(role_id=role_id).delete()
         # 准备批量插入数据
        role_menus = [
            SysRoleMenu(role_id=role_id, menu_id=menuId)
            for menuId in menuIdList
        ]
        SysRoleMenu.objects.bulk_create(role_menus)
        return Response_200()
    @action(detail=False,methods=['delete'],url_path="deleteMenuById")
    def deleteMenuById(self,request):
        menuId = request.data.get('id')
        SysRoleMenu.objects.filter(menu_id=menuId).delete()
        SysMenu.objects.filter(id=menuId).delete()
        return Response_200()
    

    